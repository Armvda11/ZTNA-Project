package middleware

import (
	"net/http"
	"sync"
	"time"

	domainErrors "control-plane/internal/domain/errors"
	"golang.org/x/time/rate"
)

// ipLimiter holds a per-IP token bucket.
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter is a per-key (IP or custom key) token-bucket rate limiter
// middleware. It uses a fixed-window leaky-bucket algorithm via
// golang.org/x/time/rate.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiter
	// r is the number of requests allowed per second per key.
	r rate.Limit
	// b is the burst size.
	b int
}

// NewIPRateLimiter creates a rate limiter that applies r req/s with burst b
// per source IP.
func NewIPRateLimiter(r rate.Limit, b int) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*ipLimiter),
		r:        r,
		b:        b,
	}
	// Background cleanup goroutine: evicts inactive keys every minute.
	go rl.cleanup()
	return rl
}

// Handler returns a middleware function that rate-limits by remote IP.
func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractClientIP(r)
		if !rl.allow(ip) {
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HandlerByPEP returns a middleware that rate-limits by the X-PEP-ID header
// value. Intended for use on the PEP router.
func (rl *RateLimiter) HandlerByPEP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-PEP-ID")
		if key == "" {
			// Fall back to IP if header absent. The PEPAuth middleware will
			// reject the request with 401 anyway.
			key = extractClientIP(r)
		}
		if !rl.allow(key) {
			writeError(w, domainErrors.ErrInvalidInput)
			http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	entry, ok := rl.limiters[key]
	if !ok {
		entry = &ipLimiter{limiter: rate.NewLimiter(rl.r, rl.b)}
		rl.limiters[key] = entry
	}
	entry.lastSeen = time.Now()
	allowed := entry.limiter.Allow()
	rl.mu.Unlock()
	return allowed
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		threshold := time.Now().Add(-5 * time.Minute)
		rl.mu.Lock()
		for k, v := range rl.limiters {
			if v.lastSeen.Before(threshold) {
				delete(rl.limiters, k)
			}
		}
		rl.mu.Unlock()
	}
}

func extractClientIP(r *http.Request) string {
	// Prefer X-Real-IP (set by trusted proxies) then fall back to RemoteAddr.
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return real
	}
	// RemoteAddr is "host:port"; strip the port.
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			host = host[:i]
			break
		}
	}
	return host
}
