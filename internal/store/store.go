// Package store is the persistence layer for ShortLink. It maps an
// auto-incrementing numeric ID to a URL and back, backed by SQLite via the
// pure-Go (CGO-free) modernc.org/sqlite driver. It is intentionally unaware of
// HTTP: it deals only in IDs and URLs.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned by GetURL when no row exists for the given ID.
var ErrNotFound = errors.New("store: not found")

// schema is idempotent so opening an existing database is a no-op. The UNIQUE
// constraint on url enforces idempotency: encoding the same URL twice yields
// the same ID (see Save).
const schema = `
CREATE TABLE IF NOT EXISTS links (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    url        TEXT    NOT NULL UNIQUE,
    created_at TEXT    NOT NULL DEFAULT (datetime('now'))
);`

// Store is a SQLite-backed link store. Safe for concurrent use: *sql.DB
// manages its own connection pool.
type Store struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path and ensures
// the schema exists. Pass ":memory:" for an ephemeral database in tests.
func Open(path string) (*Store, error) {
	// Pragmas, in order:
	//   journal_mode(WAL)   — readers don't block the single writer (and vice
	//                         versa), which is what makes concurrent encode +
	//                         redirect traffic safe on one node. WAL persists on
	//                         the file; it is a no-op for ":memory:".
	//   busy_timeout(5000)  — a writer that still finds the DB locked retries for
	//                         up to 5s instead of failing immediately.
	//   synchronous(NORMAL) — the recommended durability level under WAL: safe
	//                         across app crashes, fsync only at checkpoints.
	//   foreign_keys(1)     — good hygiene.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

// Save persists url and returns its ID. It is idempotent: saving a URL that
// already exists returns the existing ID without creating a duplicate row.
// All queries are parameterized to prevent SQL injection.
func (s *Store) Save(ctx context.Context, url string) (uint64, error) {
	// ON CONFLICT DO UPDATE (rather than DO NOTHING) guarantees RETURNING
	// yields a row on both insert and conflict paths. The no-op update keeps
	// the original created_at.
	const q = `
INSERT INTO links (url) VALUES (?)
ON CONFLICT(url) DO UPDATE SET url = excluded.url
RETURNING id;`
	var id uint64
	if err := s.db.QueryRowContext(ctx, q, url).Scan(&id); err != nil {
		return 0, fmt.Errorf("save url: %w", err)
	}
	return id, nil
}

// GetURL returns the URL for id, or ErrNotFound if no such row exists.
func (s *Store) GetURL(ctx context.Context, id uint64) (string, error) {
	const q = `SELECT url FROM links WHERE id = ?;`
	var url string
	switch err := s.db.QueryRowContext(ctx, q, id).Scan(&url); {
	case errors.Is(err, sql.ErrNoRows):
		return "", ErrNotFound
	case err != nil:
		return "", fmt.Errorf("get url: %w", err)
	default:
		return url, nil
	}
}
