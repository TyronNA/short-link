// Package handler implements the HTTP layer for ShortLink: JSON request
// parsing, input validation, response shaping, and security hardening. It
// depends on a consumer-side Store interface (not a concrete type) so it can
// be unit-tested with a mock.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/tyronnguyen/short-link/internal/shortener"
	"github.com/tyronnguyen/short-link/internal/store"
)

const (
	// maxURLLen bounds accepted URLs (RULES §4).
	maxURLLen = 2048
	// maxBodyBytes caps request bodies to mitigate memory-exhaustion DoS.
	maxBodyBytes = 16 * 1024
)

// Store is the persistence contract the handler needs. Defined here
// (consumer side) so tests can supply a mock.
type Store interface {
	Save(ctx context.Context, rawURL string) (uint64, error)
	GetURL(ctx context.Context, id uint64) (string, error)
}

// Codec converts between numeric IDs and short codes. Implemented by
// *shortener.Shortener.
type Codec interface {
	Encode(id uint64) (string, error)
	Decode(code string) (uint64, error)
}

// Handler wires the store and codec into HTTP endpoints.
type Handler struct {
	store   Store
	codec   Codec
	baseURL string // e.g. "http://localhost:8080", no trailing slash
}

// New constructs a Handler. baseURL is used to build returned short URLs;
// any trailing slash is trimmed.
func New(s Store, c Codec, baseURL string) *Handler {
	return &Handler{store: s, codec: c, baseURL: strings.TrimRight(baseURL, "/")}
}

type encodeRequest struct {
	URL string `json:"url"`
}

type encodeResponse struct {
	ShortURL string `json:"short_url"`
}

type decodeRequest struct {
	ShortURL string `json:"short_url"`
}

type decodeResponse struct {
	OriginalURL string `json:"original_url"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (h *Handler) handleEncode(w http.ResponseWriter, r *http.Request) {
	var req encodeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validateURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := h.store.Save(r.Context(), req.URL)
	if err != nil {
		log.Printf("encode: store.Save failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not store url")
		return
	}
	code, err := h.codec.Encode(id)
	if err != nil {
		// Reaching MaxID means the keyspace is exhausted — a server condition.
		log.Printf("encode: codec.Encode(id=%d) failed: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not generate code")
		return
	}
	writeJSON(w, http.StatusCreated, encodeResponse{ShortURL: h.baseURL + "/" + code})
}

func (h *Handler) handleDecode(w http.ResponseWriter, r *http.Request) {
	var req decodeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	code := extractCode(req.ShortURL)
	url, ok := h.resolve(r.Context(), w, code)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, decodeResponse{OriginalURL: url})
}

func (h *Handler) handleRedirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	url, ok := h.resolve(r.Context(), w, code)
	if !ok {
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// resolve validates a code, decodes it to an ID, and looks up the URL. On any
// failure it writes the appropriate error response and returns ok=false.
func (h *Handler) resolve(ctx context.Context, w http.ResponseWriter, code string) (string, bool) {
	// codec.Decode validates the code's format (length + base62 charset) via
	// fromBase62 before deobfuscating, so malformed input never reaches the
	// Feistel transform; it surfaces as ErrInvalidCode below.
	id, err := h.codec.Decode(code)
	if err != nil {
		if errors.Is(err, shortener.ErrInvalidCode) {
			writeError(w, http.StatusBadRequest, "invalid short code")
			return "", false
		}
		log.Printf("decode: codec.Decode(%q) failed: %v", code, err)
		writeError(w, http.StatusInternalServerError, "could not decode")
		return "", false
	}
	url, err := h.store.GetURL(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "short code not found")
			return "", false
		}
		log.Printf("decode: store.GetURL(%d) failed: %v", id, err)
		writeError(w, http.StatusInternalServerError, "could not look up url")
		return "", false
	}
	return url, true
}

// extractCode pulls the short code from either a bare code ("GeAi9K") or a
// full short URL ("http://host/GeAi9K"). It returns the last non-empty path
// segment. Invalid codes are rejected downstream by the codec.
func extractCode(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// validateURL enforces the input contract: non-empty, length-bounded, valid
// http/https URL with a host, and free of control characters (which could
// enable header/log injection — CRLF — or break clients).
func validateURL(raw string) error {
	if raw == "" {
		return errors.New("url is required")
	}
	if len(raw) > maxURLLen {
		return errors.New("url exceeds maximum length of 2048")
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return errors.New("url contains control characters")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("url is malformed")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url scheme must be http or https")
	}
	if u.Host == "" {
		return errors.New("url must include a host")
	}
	return nil
}

// decodeJSON reads and strictly decodes a JSON body, applying a body-size
// limit. It writes a 400 response and returns false on any failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// writeJSON serializes v as JSON with the given status and security headers.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	// Status is already committed; encode errors can only be logged.
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: encode failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
