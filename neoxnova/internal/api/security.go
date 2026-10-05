package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// SecurityHeaders sets conservative response headers for an API: no MIME
// sniffing, no framing/embedding, no referrer leakage, and a locked-down CSP.
// HSTS is only advertised over secure (non-development) deployments.
func SecurityHeaders(secure bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "()")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		if secure {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// ipLimiter is a small per-IP token bucket. It slows brute-force login, bulk
// registration and crude automation; it is not a substitute for real anti-bot
// measures but raises the cost of naive scripted abuse.
type ipLimiter struct {
	mu          sync.Mutex
	ratePerSec  float64
	burst       float64
	entries     map[string]*limEntry
	lastCleanup time.Time
}

type limEntry struct {
	tokens float64
	last   time.Time
}

func newIPLimiter(ratePerSec, burst float64) *ipLimiter {
	return &ipLimiter{
		ratePerSec: ratePerSec,
		burst:      burst,
		entries:    make(map[string]*limEntry),
	}
}

func (l *ipLimiter) allow(ip string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// Opportunistic cleanup so the map cannot grow unbounded.
	if now.Sub(l.lastCleanup) > 10*time.Minute {
		for k, e := range l.entries {
			if now.Sub(e.last) > 10*time.Minute {
				delete(l.entries, k)
			}
		}
		l.lastCleanup = now
	}

	e := l.entries[ip]
	if e == nil {
		e = &limEntry{tokens: l.burst, last: now}
		l.entries[ip] = e
	}
	e.tokens += now.Sub(e.last).Seconds() * l.ratePerSec
	if e.tokens > l.burst {
		e.tokens = l.burst
	}
	e.last = now
	if e.tokens < 1 {
		return false
	}
	e.tokens--
	return true
}

// Middleware rejects requests from an IP that has exhausted its bucket.
func (l *ipLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded; slow down"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP uses the connection address only. X-Forwarded-For is deliberately
// ignored because trusting it without a vetted reverse proxy lets clients spoof
// their identity and bypass rate limits.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
