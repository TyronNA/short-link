package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tyronnguyen/short-link/internal/shortener"
	"github.com/tyronnguyen/short-link/internal/store"
)

// mockStore is an in-memory Store for handler unit tests.
type mockStore struct {
	byURL   map[string]uint64
	byID    map[uint64]string
	nextID  uint64
	saveErr error
	getErr  error
}

func newMockStore() *mockStore {
	return &mockStore{byURL: map[string]uint64{}, byID: map[uint64]string{}, nextID: 1}
}

func (m *mockStore) Save(_ context.Context, rawURL string) (uint64, error) {
	if m.saveErr != nil {
		return 0, m.saveErr
	}
	if id, ok := m.byURL[rawURL]; ok {
		return id, nil
	}
	id := m.nextID
	m.nextID++
	m.byURL[rawURL] = id
	m.byID[id] = rawURL
	return id, nil
}

func (m *mockStore) GetURL(_ context.Context, id uint64) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	if u, ok := m.byID[id]; ok {
		return u, nil
	}
	return "", store.ErrNotFound
}

func newTestHandler(s Store) http.Handler {
	return New(s, shortener.New(0xABCDEF), "http://short.test").Routes()
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEncodeHappyPath(t *testing.T) {
	h := newTestHandler(newMockStore())
	rec := doJSON(t, h, "POST", "/encode", `{"url":"https://example.com/foo"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if ns := rec.Header().Get("X-Content-Type-Options"); ns != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", ns)
	}
	var resp encodeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.HasPrefix(resp.ShortURL, "http://short.test/") {
		t.Errorf("short_url = %q, missing base", resp.ShortURL)
	}
	code := resp.ShortURL[len("http://short.test/"):]
	if len(code) != shortener.CodeLength {
		t.Errorf("code %q length = %d, want %d", code, len(code), shortener.CodeLength)
	}
}

func TestEncodeIdempotent(t *testing.T) {
	h := newTestHandler(newMockStore())
	r1 := doJSON(t, h, "POST", "/encode", `{"url":"https://example.com/same"}`)
	r2 := doJSON(t, h, "POST", "/encode", `{"url":"https://example.com/same"}`)
	if r1.Body.String() != r2.Body.String() {
		t.Errorf("idempotency broken: %q vs %q", r1.Body.String(), r2.Body.String())
	}
}

func TestEncodeValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"empty body", ``, http.StatusBadRequest},
		{"invalid json", `{`, http.StatusBadRequest},
		{"unknown field", `{"foo":"bar"}`, http.StatusBadRequest},
		{"missing url", `{"url":""}`, http.StatusBadRequest},
		{"bad scheme", `{"url":"ftp://example.com"}`, http.StatusBadRequest},
		{"no host", `{"url":"http://"}`, http.StatusBadRequest},
		{"not a url", `{"url":"just text"}`, http.StatusBadRequest},
		{"too long", `{"url":"https://example.com/` + strings.Repeat("a", 2048) + `"}`, http.StatusBadRequest},
		{"control char", "{\"url\":\"https://example.com/\n\"}", http.StatusBadRequest},
	}
	h := newTestHandler(newMockStore())
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, h, "POST", "/encode", tc.body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, tc.want, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("error response Content-Type = %q, want application/json", ct)
			}
		})
	}
}

func TestEncodeBodyTooLarge(t *testing.T) {
	h := newTestHandler(newMockStore())
	big := `{"url":"https://example.com/` + strings.Repeat("a", 20*1024) + `"}`
	rec := doJSON(t, h, "POST", "/encode", big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}

func TestEncodeStoreError(t *testing.T) {
	ms := newMockStore()
	ms.saveErr = errors.New("db down")
	h := newTestHandler(ms)
	rec := doJSON(t, h, "POST", "/encode", `{"url":"https://example.com/x"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "db down") {
		t.Error("internal error leaked to client")
	}
}

func TestDecodeRoundTrip(t *testing.T) {
	h := newTestHandler(newMockStore())
	enc := doJSON(t, h, "POST", "/encode", `{"url":"https://example.com/roundtrip"}`)
	var er encodeResponse
	json.Unmarshal(enc.Body.Bytes(), &er)

	dec := doJSON(t, h, "POST", "/decode", `{"short_url":"`+er.ShortURL+`"}`)
	if dec.Code != http.StatusOK {
		t.Fatalf("decode status = %d, want 200; body=%s", dec.Code, dec.Body)
	}
	var dr decodeResponse
	if err := json.Unmarshal(dec.Body.Bytes(), &dr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dr.OriginalURL != "https://example.com/roundtrip" {
		t.Errorf("original_url = %q, want original", dr.OriginalURL)
	}
}

func TestDecodeBareCode(t *testing.T) {
	h := newTestHandler(newMockStore())
	enc := doJSON(t, h, "POST", "/encode", `{"url":"https://example.com/bare"}`)
	var er encodeResponse
	json.Unmarshal(enc.Body.Bytes(), &er)
	code := er.ShortURL[len("http://short.test/"):]

	dec := doJSON(t, h, "POST", "/decode", `{"short_url":"`+code+`"}`)
	if dec.Code != http.StatusOK {
		t.Fatalf("decode of bare code status = %d, want 200", dec.Code)
	}
}

func TestDecodeErrors(t *testing.T) {
	h := newTestHandler(newMockStore())
	cases := []struct {
		name string
		body string
		want int
	}{
		{"invalid json", `{`, http.StatusBadRequest},
		{"bad code length", `{"short_url":"http://short.test/abc"}`, http.StatusBadRequest},
		{"bad charset", `{"short_url":"http://short.test/ab-d!f"}`, http.StatusBadRequest},
		{"unknown code", `{"short_url":"http://short.test/ZZZZZZ"}`, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, h, "POST", "/decode", tc.body)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d; body=%s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestRedirect(t *testing.T) {
	h := newTestHandler(newMockStore())
	enc := doJSON(t, h, "POST", "/encode", `{"url":"https://example.com/redir"}`)
	var er encodeResponse
	json.Unmarshal(enc.Body.Bytes(), &er)
	code := er.ShortURL[len("http://short.test/"):]

	req := httptest.NewRequest("GET", "/"+code, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("redirect status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://example.com/redir" {
		t.Errorf("Location = %q, want original url", loc)
	}
}

func TestRedirectInvalidCode(t *testing.T) {
	h := newTestHandler(newMockStore())
	req := httptest.NewRequest("GET", "/bad", nil) // length != 6
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHealth(t *testing.T) {
	h := newTestHandler(newMockStore())
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestExtractCode(t *testing.T) {
	cases := map[string]string{
		"GeAi9K":                       "GeAi9K",
		"http://short.test/GeAi9K":     "GeAi9K",
		"http://short.test/GeAi9K/":    "GeAi9K",
		"https://x.y/a/b/GeAi9K":       "GeAi9K",
		"  http://short.test/GeAi9K  ": "GeAi9K",
	}
	for in, want := range cases {
		if got := extractCode(in); got != want {
			t.Errorf("extractCode(%q) = %q, want %q", in, got, want)
		}
	}
}
