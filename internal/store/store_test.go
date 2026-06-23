package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestSaveAndGet(t *testing.T) {
	s, _ := tempStore(t)
	ctx := context.Background()

	id, err := s.Save(ctx, "https://example.com/page")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if id == 0 {
		t.Fatal("Save returned id 0; AUTOINCREMENT starts at 1")
	}

	url, err := s.GetURL(ctx, id)
	if err != nil {
		t.Fatalf("GetURL: %v", err)
	}
	if url != "https://example.com/page" {
		t.Errorf("GetURL = %q, want %q", url, "https://example.com/page")
	}
}

func TestSaveIdempotent(t *testing.T) {
	s, _ := tempStore(t)
	ctx := context.Background()

	id1, err := s.Save(ctx, "https://example.com/same")
	if err != nil {
		t.Fatalf("Save #1: %v", err)
	}
	id2, err := s.Save(ctx, "https://example.com/same")
	if err != nil {
		t.Fatalf("Save #2: %v", err)
	}
	if id1 != id2 {
		t.Errorf("idempotency broken: id1=%d id2=%d", id1, id2)
	}

	// A different URL must get a different ID.
	id3, err := s.Save(ctx, "https://example.com/other")
	if err != nil {
		t.Fatalf("Save #3: %v", err)
	}
	if id3 == id1 {
		t.Errorf("distinct URLs shared id %d", id3)
	}
}

func TestGetNotFound(t *testing.T) {
	s, _ := tempStore(t)
	_, err := s.GetURL(context.Background(), 99999)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetURL(missing) error = %v, want ErrNotFound", err)
	}
}

// TestPersistAcrossRestart proves R6: encoded URLs are recoverable after the
// process (here, the *sql.DB handle) is closed and the same file is reopened.
func TestPersistAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	ctx := context.Background()

	s1, err := Open(path)
	if err != nil {
		t.Fatalf("Open #1: %v", err)
	}
	id, err := s1.Save(ctx, "https://persist.example/resource")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen the same file — simulates an application restart.
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	defer s2.Close()

	url, err := s2.GetURL(ctx, id)
	if err != nil {
		t.Fatalf("GetURL after restart: %v", err)
	}
	if url != "https://persist.example/resource" {
		t.Errorf("after restart GetURL = %q, want original URL", url)
	}
}

// TestOpenBadPath exercises the schema-initialization failure branch: a path
// inside a non-existent directory cannot be created, so opening fails.
func TestOpenBadPath(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "no-such-dir", "nested", "x.db"))
	if err == nil {
		t.Fatal("Open with unwritable path: expected error, got nil")
	}
}

// TestSaveURLWithQuote ensures parameterized queries handle a single quote
// (a classic SQL-injection probe) without breaking.
func TestSaveURLWithQuote(t *testing.T) {
	s, _ := tempStore(t)
	ctx := context.Background()
	tricky := "https://example.com/a'); DROP TABLE links;--"

	id, err := s.Save(ctx, tricky)
	if err != nil {
		t.Fatalf("Save tricky URL: %v", err)
	}
	got, err := s.GetURL(ctx, id)
	if err != nil {
		t.Fatalf("GetURL: %v", err)
	}
	if got != tricky {
		t.Errorf("GetURL = %q, want %q", got, tricky)
	}
	// The table must still exist and be usable.
	if _, err := s.Save(ctx, "https://example.com/after"); err != nil {
		t.Fatalf("table unusable after injection probe: %v", err)
	}
}
