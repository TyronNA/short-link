// Package e2e exercises the full stack — real HTTP server, real codec, real
// SQLite on a temp file — including the restart-and-decode scenario (R6).
package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tyronnguyen/short-link/internal/handler"
	"github.com/tyronnguyen/short-link/internal/shortener"
	"github.com/tyronnguyen/short-link/internal/store"
)

func startServer(t *testing.T, dbPath string) (*httptest.Server, func()) {
	t.Helper()
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	codec := shortener.New(0x5151515151)
	// NewUnstartedServer lets us learn the listen address (srv.URL) before
	// building the handler, so the returned short URLs carry the real base URL.
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	srv.Config.Handler = handler.New(st, codec, srv.URL).Routes()

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			srv.Close()
			st.Close()
		})
	}
	return srv, cleanup
}

func postJSON(t *testing.T, url string, body any) (*http.Response, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func TestFullFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "e2e.db")
	srv, cleanup := startServer(t, dbPath)
	defer cleanup()

	const original = "https://codesubmit.io/library/react"

	// 1. Encode.
	resp, body := postJSON(t, srv.URL+"/encode", map[string]string{"url": original})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("encode status = %d, body=%s", resp.StatusCode, body)
	}
	var enc struct {
		ShortURL string `json:"short_url"`
	}
	if err := json.Unmarshal(body, &enc); err != nil {
		t.Fatalf("unmarshal encode: %v", err)
	}
	code := enc.ShortURL[len(srv.URL)+1:]
	if len(code) != shortener.CodeLength {
		t.Fatalf("code %q length = %d, want %d", code, len(code), shortener.CodeLength)
	}

	// 2. Decode round-trip.
	resp, body = postJSON(t, srv.URL+"/decode", map[string]string{"short_url": enc.ShortURL})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("decode status = %d, body=%s", resp.StatusCode, body)
	}
	var dec struct {
		OriginalURL string `json:"original_url"`
	}
	json.Unmarshal(body, &dec)
	if dec.OriginalURL != original {
		t.Fatalf("decoded %q, want %q", dec.OriginalURL, original)
	}

	// 3. Redirect (no auto-follow).
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	rresp, err := client.Get(enc.ShortURL)
	if err != nil {
		t.Fatalf("GET redirect: %v", err)
	}
	rresp.Body.Close()
	if rresp.StatusCode != http.StatusFound {
		t.Fatalf("redirect status = %d, want 302", rresp.StatusCode)
	}
	if loc := rresp.Header.Get("Location"); loc != original {
		t.Fatalf("Location = %q, want %q", loc, original)
	}

	// 4. Idempotency: encoding the same URL returns the same short URL.
	_, body2 := postJSON(t, srv.URL+"/encode", map[string]string{"url": original})
	var enc2 struct {
		ShortURL string `json:"short_url"`
	}
	json.Unmarshal(body2, &enc2)
	if enc2.ShortURL != enc.ShortURL {
		t.Fatalf("idempotency: %q != %q", enc2.ShortURL, enc.ShortURL)
	}

	// 5. Unknown code → 404.
	resp, _ = postJSON(t, srv.URL+"/decode", map[string]string{"short_url": "ZzZzZz"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown code status = %d, want 404", resp.StatusCode)
	}

	// 6. Restart: close server+store, reopen the SAME db file, decode again.
	cleanup()
	srv2, cleanup2 := startServer(t, dbPath)
	defer cleanup2()

	// The code embeds an ID independent of base URL, so rebuild the decode
	// payload against the new server but with the original code.
	resp, body = postJSON(t, srv2.URL+"/decode", map[string]string{"short_url": code})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("decode after restart status = %d, body=%s", resp.StatusCode, body)
	}
	json.Unmarshal(body, &dec)
	if dec.OriginalURL != original {
		t.Fatalf("after restart decoded %q, want %q (R6 persistence failed)", dec.OriginalURL, original)
	}
}
