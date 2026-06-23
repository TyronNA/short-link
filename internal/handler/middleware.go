package handler

import (
	"net/http"
	"strings"
)

// CORS wraps next with Cross-Origin Resource Sharing handling for the given
// allowed origins. It is needed because the static front-end is served from a
// different origin (e.g. https://short-link.fun) than this API
// (https://api.short-link.fun).
//
// allowed may contain "*" to allow any origin. A wildcard is safe here because
// the API uses no cookies or credentials. Otherwise only exact-match origins
// are reflected. Preflight (OPTIONS) requests are answered directly.
//
// If allowed is empty, no CORS headers are emitted (same-origin / CORS off).
func CORS(next http.Handler, allowed []string) http.Handler {
	allowAll := false
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			allowAll = true
		}
		set[o] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if _, ok := set[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
		}

		// Preflight: a CORS OPTIONS probe carries Access-Control-Request-Method.
		// The routing mux only knows GET/POST, so it would 405 these; answer here.
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
