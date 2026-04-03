package web

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter implements a per-IP token bucket rate limiter.
type rateLimiter struct {
	mu       sync.Mutex
	clients  map[string]*bucket
	rate     int // tokens per interval
	interval time.Duration
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

func newRateLimiter(rate int, interval time.Duration, ctx context.Context) *rateLimiter {
	rl := &rateLimiter{
		clients:  make(map[string]*bucket),
		rate:     rate,
		interval: interval,
	}
	// Periodically clean up stale entries; exits when context is canceled.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rl.cleanup()
			case <-ctx.Done():
				return
			}
		}
	}()
	return rl
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.clients[ip]
	now := time.Now()

	if !ok {
		rl.clients[ip] = &bucket{tokens: rl.rate - 1, lastReset: now}
		return true
	}

	// Reset tokens if interval has passed
	if now.Sub(b.lastReset) >= rl.interval {
		b.tokens = rl.rate
		b.lastReset = now
	}

	if b.tokens <= 0 {
		return false
	}

	b.tokens--
	return true
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-10 * time.Minute)
	for ip, b := range rl.clients {
		if b.lastReset.Before(cutoff) {
			delete(rl.clients, ip)
		}
	}
}

// clientIP extracts the client IP from RemoteAddr only.
// Proxy headers (X-Forwarded-For, X-Real-IP) are NOT trusted because they
// are trivially spoofable and would allow rate-limit bypass.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
