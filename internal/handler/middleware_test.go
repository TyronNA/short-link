package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler is a trivial next-handler used to confirm CORS passes through.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
})

func TestCORS_AllowlistMatch(t *testing.T) {
	h := CORS(okHandler, []string{"https://short-link.fun"})

	req := httptest.NewRequest(http.MethodPost, "/encode", nil)
	req.Header.Set("Origin", "https://short-link.fun")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://short-link.fun" {
		t.Fatalf("ACAO = %q, want reflected origin", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (passed through to next)", rec.Code)
	}
}

func TestCORS_AllowlistNoMatch(t *testing.T) {
	h := CORS(okHandler, []string{"https://short-link.fun"})

	req := httptest.NewRequest(http.MethodPost, "/encode", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("ACAO = %q, want empty for disallowed origin", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (request still served)", rec.Code)
	}
}

func TestCORS_Wildcard(t *testing.T) {
	h := CORS(okHandler, []string{"*"})

	req := httptest.NewRequest(http.MethodPost, "/encode", nil)
	req.Header.Set("Origin", "https://anything.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("ACAO = %q, want *", got)
	}
}

func TestCORS_Preflight(t *testing.T) {
	h := CORS(okHandler, []string{"https://short-link.fun"})

	req := httptest.NewRequest(http.MethodOptions, "/encode", nil)
	req.Header.Set("Origin", "https://short-link.fun")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("missing Access-Control-Allow-Methods on preflight")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://short-link.fun" {
		t.Errorf("ACAO = %q, want reflected origin on preflight", got)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("preflight body = %q, want empty (next not called)", rec.Body.String())
	}
}

func TestCORS_NoOriginNoHeaders(t *testing.T) {
	h := CORS(okHandler, []string{"*"})

	req := httptest.NewRequest(http.MethodGet, "/health", nil) // no Origin header
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q, want empty when no Origin sent", got)
	}
}

func TestCORS_EmptyAllowlistDisabled(t *testing.T) {
	h := CORS(okHandler, nil)

	req := httptest.NewRequest(http.MethodPost, "/encode", nil)
	req.Header.Set("Origin", "https://short-link.fun")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO = %q, want empty when CORS disabled", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
