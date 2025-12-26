package srv

import (
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RateLimiter implements a simple token bucket rate limiter per IP.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int           // tokens per interval
	interval time.Duration // refill interval
	burst    int           // max tokens
}

type visitor struct {
	tokens   int
	lastSeen time.Time
}

// NewRateLimiter creates a rate limiter that allows `rate` requests per `interval`
// with a burst capacity of `burst`.
func NewRateLimiter(rate int, interval time.Duration, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		interval: interval,
		burst:    burst,
	}
	// Cleanup stale entries every minute
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	for {
		time.Sleep(time.Minute)
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > 5*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow checks if a request from the given IP should be allowed.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	now := time.Now()

	if !exists {
		rl.visitors[ip] = &visitor{tokens: rl.burst - 1, lastSeen: now}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(v.lastSeen)
	refill := int(elapsed / rl.interval) * rl.rate
	v.tokens += refill
	if v.tokens > rl.burst {
		v.tokens = rl.burst
	}
	v.lastSeen = now

	if v.tokens > 0 {
		v.tokens--
		return true
	}
	return false
}

// getRateLimitKey returns the key to use for rate limiting.
// For Nightbot requests, use channel name; otherwise use IP.
func getRateLimitKey(r *http.Request) (key string, keyType string) {
	// Check for Nightbot-Channel header first
	if channelHeader := r.Header.Get("Nightbot-Channel"); channelHeader != "" {
		if channel := ParseNightbotChannel(channelHeader); channel != nil && channel.Name != "" {
			return "channel:" + channel.Name, "channel"
		}
	}

	// Fall back to IP-based rate limiting
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	return "ip:" + ip, "ip"
}

// Middleware wraps an http.Handler with rate limiting.
// Uses per-channel rate limiting for Nightbot requests, per-IP otherwise.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, keyType := getRateLimitKey(r)

		if !rl.Allow(key) {
			// Record rate limit event on span
			span := trace.SpanFromContext(r.Context())
			span.AddEvent("rate_limited", trace.WithAttributes(
				attribute.String("rate_limit.key", key),
				attribute.String("rate_limit.key_type", keyType),
				attribute.String("endpoint", r.URL.Path),
			))
			http.Error(w, "Rate limit exceeded. Try again later.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
