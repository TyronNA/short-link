package handler

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimiterConfig configures the per-client token-bucket rate limiter.
type RateLimiterConfig struct {
	// RPS is the sustained requests-per-second allowed per client. A value <= 0
	// disables limiting entirely (Middleware returns next unchanged).
	RPS float64
	// Burst is the maximum number of requests a client may make in an instant
	// (the bucket capacity). It should be >= 1 when RPS > 0.
	Burst float64
	// TrustProxy makes the limiter key on the X-Forwarded-For / X-Real-IP header
	// (left-most entry) instead of the TCP peer address. Enable ONLY when the
	// service runs behind a trusted reverse proxy that sets these headers
	// (e.g. Caddy/Cloudflare); otherwise clients can spoof the header to evade
	// the limit. When behind a proxy you MUST enable it, else every request
	// shares the proxy's IP and throttles all clients together.
	TrustProxy bool

	// now and gcInterval are injectable for tests; zero values use real time.
	now        func() time.Time
	gcInterval time.Duration
}

// RateLimiter is an in-memory, per-client token-bucket limiter. It is safe for
// concurrent use. A background janitor evicts buckets idle longer than the
// refill time so memory stays bounded under churn; call Stop to release it.
type RateLimiter struct {
	rps   float64
	burst float64
	trust bool
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	idleTTL time.Duration

	stop chan struct{}
	once sync.Once
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewRateLimiter builds a limiter from cfg and starts its janitor goroutine.
// The caller should defer Stop. If cfg.RPS <= 0 it still returns a usable
// (allow-all) limiter, but Middleware will bypass it.
func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	nowFn := cfg.now
	if nowFn == nil {
		nowFn = time.Now
	}
	gc := cfg.gcInterval
	if gc <= 0 {
		gc = time.Minute
	}
	// Evict a bucket once it has been idle long enough to have fully refilled
	// (so re-creating it lazily gives the client the same full burst), with a
	// floor so a high RPS doesn't cause churny eviction.
	idleTTL := 10 * time.Minute
	if cfg.RPS > 0 {
		if refill := time.Duration(cfg.Burst/cfg.RPS) * time.Second; refill > idleTTL {
			idleTTL = refill
		}
	}

	rl := &RateLimiter{
		rps:     cfg.RPS,
		burst:   cfg.Burst,
		trust:   cfg.TrustProxy,
		now:     nowFn,
		buckets: make(map[string]*bucket),
		idleTTL: idleTTL,
		stop:    make(chan struct{}),
	}
	go rl.janitor(gc)
	return rl
}

// Stop terminates the janitor goroutine. Safe to call more than once.
func (rl *RateLimiter) Stop() {
	rl.once.Do(func() { close(rl.stop) })
}

// allow reports whether a request from key may proceed, consuming a token if so.
func (rl *RateLimiter) allow(key string) bool {
	now := rl.now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		// New client starts with a full bucket, minus the request it is making.
		rl.buckets[key] = &bucket{tokens: rl.burst - 1, last: now}
		return true
	}

	// Refill based on elapsed time, capped at burst.
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = min(rl.burst, b.tokens+elapsed*rl.rps)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (rl *RateLimiter) janitor(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-rl.stop:
			return
		case <-t.C:
			cutoff := rl.now().Add(-rl.idleTTL)
			rl.mu.Lock()
			for k, b := range rl.buckets {
				if b.last.Before(cutoff) {
					delete(rl.buckets, k)
				}
			}
			rl.mu.Unlock()
		}
	}
}

// Middleware wraps next, rejecting requests from a client that has exceeded its
// budget with 429 and a Retry-After header. /health is never limited so health
// probes are not throttled. If limiting is disabled (RPS <= 0), next is
// returned unchanged.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	if rl.rps <= 0 {
		return next
	}
	retryAfter := strconv.Itoa(max(1, int(1.0/rl.rps)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		if !rl.allow(rl.clientKey(r)) {
			w.Header().Set("Retry-After", retryAfter)
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientKey derives the rate-limit bucket key for r. Behind a trusted proxy it
// uses the originating client IP from X-Forwarded-For / X-Real-IP; otherwise it
// uses the TCP peer address.
func (rl *RateLimiter) clientKey(r *http.Request) string {
	if rl.trust {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
				return ip
			}
		}
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
