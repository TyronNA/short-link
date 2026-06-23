package handler

import "net/http"

// Routes returns a ServeMux with all endpoints registered. Keeping the route
// table in one place separates "what is exposed" from the handler logic in
// handler.go. Method-specific patterns require Go 1.22+.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /encode", h.handleEncode)
	mux.HandleFunc("POST /decode", h.handleDecode)
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("GET /{code}", h.handleRedirect)
	return mux
}
