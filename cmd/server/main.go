// Command server is the ShortLink HTTP service entrypoint. It only wires
// configuration, store, codec, and HTTP server together, then runs with
// graceful shutdown. All logic lives in internal/*.
package main

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tyronnguyen/short-link/internal/handler"
	"github.com/tyronnguyen/short-link/internal/shortener"
	"github.com/tyronnguyen/short-link/internal/store"
)

type config struct {
	port        string
	dbPath      string
	baseURL     string
	feistelKey  uint64
	corsOrigins []string
}

func loadConfig() config {
	port := env("PORT", "8080")
	return config{
		port:        port,
		dbPath:      env("DB_PATH", "shortlink.db"),
		baseURL:     env("BASE_URL", "http://localhost:"+port),
		feistelKey:  deriveKey(env("FEISTEL_KEY", "shortlink-default-key")),
		corsOrigins: splitCSV(env("CORS_ALLOWED_ORIGINS", "")),
	}
}

// splitCSV splits a comma-separated env value into trimmed, non-empty entries.
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// deriveKey turns a human-readable secret into a 64-bit Feistel key.
func deriveKey(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg := loadConfig()

	st, err := store.Open(cfg.dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	codec := shortener.New(cfg.feistelKey)
	h := handler.New(st, codec, cfg.baseURL)

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           handler.CORS(h.Routes(), cfg.corsOrigins),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Run the server in a goroutine so main can wait for a shutdown signal.
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("listening on %s (base_url=%s, db=%s, cors=%v)", srv.Addr, cfg.baseURL, cfg.dbPath, cfg.corsOrigins)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErr:
		return fmt.Errorf("server: %w", err)
	case <-ctx.Done():
		log.Println("shutdown signal received; draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Println("stopped cleanly")
	return nil
}
