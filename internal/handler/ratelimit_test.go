package handler

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// fakeClock is a controllable time source for deterministic limiter tests.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestLimiter(cfg RateLimiterConfig, clk *fakeClock) *RateLimiter {
	cfg.now = clk.now
	cfg.gcInterval = time.Hour // keep the janitor out of the way in tests
	return NewRateLimiter(cfg)
}

func TestRateLimiterBurstThenReject(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	rl := newTestLimiter(RateLimiterConfig{RPS: 1, Burst: 3}, clk)
	defer rl.Stop()
	h := rl.Middleware(okHandler)

	req := func() int {
		r := httptest.NewRequest(http.MethodGet, "/abc123", nil)
		r.RemoteAddr = "10.0.0.1:5555"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	// Burst of 3 is allowed; the 4th (no time elapsed) is rejected.
	for i := 0; i < 3; i++ {
		if got := req(); got != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, got)
		}
	}
	if got := req(); got != http.StatusTooManyRequests {
		t.Fatalf("4th request: status = %d, want 429", got)
	}

	// After 1s, 1 token refills (RPS=1) → one more request allowed, then blocked.
	clk.advance(time.Second)
	if got := req(); got != http.StatusOK {
		t.Fatalf("after refill: status = %d, want 200", got)
	}
	if got := req(); got != http.StatusTooManyRequests {
		t.Fatalf("after refill exhausted: status = %d, want 429", got)
	}
}

func TestRateLimiterPerClientIsolation(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	rl := newTestLimiter(RateLimiterConfig{RPS: 1, Burst: 1}, clk)
	defer rl.Stop()
	h := rl.Middleware(okHandler)

	do := func(addr string) int {
		r := httptest.NewRequest(http.MethodGet, "/abc123", nil)
		r.RemoteAddr = addr
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}

	if got := do("10.0.0.1:1"); got != http.StatusOK {
		t.Fatalf("client A first: %d, want 200", got)
	}
	if got := do("10.0.0.1:1"); got != http.StatusTooManyRequests {
		t.Fatalf("client A second: %d, want 429", got)
	}
	// A different client is unaffected by A's exhausted bucket.
	if got := do("10.0.0.2:1"); got != http.StatusOK {
		t.Fatalf("client B first: %d, want 200", got)
	}
}

func TestRateLimiterHealthExempt(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	rl := newTestLimiter(RateLimiterConfig{RPS: 1, Burst: 1}, clk)
	defer rl.Stop()
	h := rl.Middleware(okHandler)

	for i := 0; i < 5; i++ {
		r := httptest.NewRequest(http.MethodGet, "/health", nil)
		r.RemoteAddr = "10.0.0.9:1"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("/health probe %d throttled: status = %d", i+1, w.Code)
		}
	}
}

func TestRateLimiterDisabled(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	rl := newTestLimiter(RateLimiterConfig{RPS: 0}, clk)
	defer rl.Stop()
	h := rl.Middleware(okHandler)

	for i := 0; i < 100; i++ {
		r := httptest.NewRequest(http.MethodGet, "/abc123", nil)
		r.RemoteAddr = "10.0.0.1:1"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("disabled limiter blocked request %d: %d", i+1, w.Code)
		}
	}
}

func TestRateLimiterTrustProxy(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	rl := newTestLimiter(RateLimiterConfig{RPS: 1, Burst: 1, TrustProxy: true}, clk)
	defer rl.Stop()

	// Same proxy peer, different originating clients via XFF → separate buckets.
	do := func(xff string) int {
		r := httptest.NewRequest(http.MethodGet, "/abc123", nil)
		r.RemoteAddr = "172.16.0.1:443" // the proxy
		r.Header.Set("X-Forwarded-For", xff)
		w := httptest.NewRecorder()
		rl.Middleware(okHandler).ServeHTTP(w, r)
		return w.Code
	}

	if got := do("1.1.1.1, 172.16.0.1"); got != http.StatusOK {
		t.Fatalf("client 1.1.1.1 first: %d, want 200", got)
	}
	if got := do("1.1.1.1"); got != http.StatusTooManyRequests {
		t.Fatalf("client 1.1.1.1 second: %d, want 429", got)
	}
	if got := do("2.2.2.2"); got != http.StatusOK {
		t.Fatalf("client 2.2.2.2 first: %d, want 200 (must not share bucket)", got)
	}
}
