package mesh

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// httpRateLimiter is a simple per-IP token bucket.
// - tokens refill at rps
// - burst caps token capacity
type httpRateLimiter struct {
	mu     sync.Mutex
	rps    float64
	burst  float64
	ttl    time.Duration
	bucket map[string]*ipBucket
}

type ipBucket struct {
	tokens float64
	last   time.Time
	seen   time.Time
}

func newHTTPRateLimiter(rps int, burst int) *httpRateLimiter {
	if rps <= 0 {
		rps = 25
	}
	if burst <= 0 {
		burst = 50
	}
	return &httpRateLimiter{
		rps:    float64(rps),
		burst:  float64(burst),
		ttl:    10 * time.Minute,
		bucket: make(map[string]*ipBucket),
	}
}

func (rl *httpRateLimiter) allow(ip string, cost float64) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// garbage collect occasionally (O(n) but bounded by ttl and small in dev)
	for k, b := range rl.bucket {
		if now.Sub(b.seen) > rl.ttl {
			delete(rl.bucket, k)
		}
	}

	b := rl.bucket[ip]
	if b == nil {
		b = &ipBucket{tokens: rl.burst, last: now, seen: now}
		rl.bucket[ip] = b
	}

	// refill
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * rl.rps
		if b.tokens > rl.burst {
			b.tokens = rl.burst
		}
		b.last = now
	}
	b.seen = now

	if cost <= 0 {
		cost = 1
	}
	if b.tokens < cost {
		return false
	}
	b.tokens -= cost
	return true
}

func clientIP(r *http.Request) string {
	// Trust direct RemoteAddr (no proxy assumptions for now).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	// fallback: raw
	return r.RemoteAddr
}

func isLoopbackIP(ip string) bool {
	// Treat localhost as internal. RemoteAddr may be "127.0.0.1" or "::1".
	// We intentionally do NOT trust X-Forwarded-For here.
	ip = strings.TrimSpace(ip)
	if ip == "127.0.0.1" || ip == "::1" {
		return true
	}
	// If something like "127.0.0.1%lo0" ever appears, handle it.
	if strings.HasPrefix(ip, "127.") {
		return true
	}
	return false
}

func rateCost(path string) float64 {
	// Cheap reads
	if strings.HasPrefix(path, "/chain/status") || strings.HasPrefix(path, "/chain/height") || strings.HasPrefix(path, "/chain/block") {
		return 1
	}
	// Sensitive writes
	if strings.HasPrefix(path, "/chain/tx") {
		return 5
	}
	if strings.HasPrefix(path, "/chain/apply") || strings.HasPrefix(path, "/chain/snapshot/apply") {
		return 8
	}
	if strings.HasPrefix(path, "/chain/propose") {
		return 10
	}
	// Default
	return 1
}

func write429(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": "rate_limited",
	})
}

// buildHTTPMiddleware returns handler wrapper based on cfg.
// Policy:
// - If config explicitly sets http_rate_limit_enabled to false => OFF.
// - If field missing (legacy) => ON.
// We approximate "missing" by: treat cfg.HttpRateLimitEnabled == false as enabled unless rps/burst were explicitly set? Not reliable.
// So we implement: if rps/burst set and enabled false => OFF; else ON.
// This keeps defaults ON for legacy configs.
func buildHTTPMiddleware(cfg *MeshConfig) func(http.Handler) http.Handler {
	rl := newHTTPRateLimiter(cfg.HttpRateLimitRPS, cfg.HttpRateLimitBurst)

	enabled := true
	// if user explicitly disabled, they will also likely include the field.
	// honor explicit disable only when rps/burst fields exist or enable flag is present in config file.
	// easiest deterministic behavior: if cfg.HttpRateLimitEnabled == false AND cfg.HttpRateLimitRPS == 0 AND cfg.HttpRateLimitBurst == 0 => legacy -> enabled.
	// if cfg.HttpRateLimitEnabled == false AND (rps/burst nonzero) => explicit disable -> disabled.
	if !cfg.HttpRateLimitEnabled && (cfg.HttpRateLimitRPS != 0 || cfg.HttpRateLimitBurst != 0) {
		enabled = false
	}
	// if explicitly enabled, keep enabled=true (default).

	if !enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			cost := rateCost(r.URL.Path)

			// LOOPBACK WRITE EXEMPT (Phase 6.19 LEGO):
			// Keep rate limits for reads (e.g. /chain/status), but never block local
			// write paths (tx/apply/propose) during dev/test on 127.0.0.1.
			if isLoopbackIP(ip) && cost >= 5 {
				next.ServeHTTP(w, r)
				return
			}
			// Only rate limit API surface; leave root empty anyway.
			if strings.HasPrefix(r.URL.Path, "/chain/") || strings.HasPrefix(r.URL.Path, "/debug/") || r.URL.Path == "/peers" || r.URL.Path == "/routes" || r.URL.Path == "/reputation" || r.URL.Path == "/uc" {
				if !rl.allow(ip, cost) {
					write429(w)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
