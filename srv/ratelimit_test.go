package srv

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// newTestRateLimiter creates a rate limiter without the cleanup goroutine
// for deterministic testing.
func newTestRateLimiter(rate int, interval time.Duration, burst int) *RateLimiter {
	return &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		interval: interval,
		burst:    burst,
	}
}

func TestRateLimiter_FirstRequestAllowed(t *testing.T) {
	rl := newTestRateLimiter(1, time.Second, 5)

	if !rl.Allow("192.168.1.1") {
		t.Error("first request should be allowed")
	}
}

func TestRateLimiter_BurstCapacity(t *testing.T) {
	burst := 5
	rl := newTestRateLimiter(1, time.Second, burst)
	ip := "192.168.1.1"

	// Should allow exactly `burst` requests
	for i := 0; i < burst; i++ {
		if !rl.Allow(ip) {
			t.Errorf("request %d should be allowed (within burst)", i+1)
		}
	}

	// Next request should be denied
	if rl.Allow(ip) {
		t.Error("request after burst exhausted should be denied")
	}
}

func TestRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := newTestRateLimiter(1, time.Second, 2)

	// Exhaust IP1's tokens
	rl.Allow("ip1")
	rl.Allow("ip1")

	if rl.Allow("ip1") {
		t.Error("ip1 should be rate limited")
	}

	// IP2 should still have full burst
	if !rl.Allow("ip2") {
		t.Error("ip2 should not be affected by ip1's rate limit")
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	// 1 token per 100ms, burst of 2
	rl := newTestRateLimiter(1, 100*time.Millisecond, 2)
	ip := "192.168.1.1"

	// Use both tokens
	rl.Allow(ip)
	rl.Allow(ip)

	if rl.Allow(ip) {
		t.Error("should be denied after burst exhausted")
	}

	// Wait for 1 token to refill
	time.Sleep(150 * time.Millisecond)

	if !rl.Allow(ip) {
		t.Error("should be allowed after token refill")
	}

	// Should be denied again
	if rl.Allow(ip) {
		t.Error("should be denied after using refilled token")
	}
}

func TestRateLimiter_RefillCapsAtBurst(t *testing.T) {
	// 10 tokens per 10ms, burst of 3
	rl := newTestRateLimiter(10, 10*time.Millisecond, 3)
	ip := "192.168.1.1"

	// Use 1 token
	rl.Allow(ip)

	// Wait long enough for many refills
	time.Sleep(100 * time.Millisecond)

	// Should only have burst capacity (3), not unlimited
	allowed := 0
	for i := 0; i < 10; i++ {
		if rl.Allow(ip) {
			allowed++
		}
	}

	if allowed != 3 {
		t.Errorf("expected 3 allowed (burst cap), got %d", allowed)
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := newTestRateLimiter(1, time.Second, 100)
	ip := "192.168.1.1"

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	// 200 concurrent requests
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed <- rl.Allow(ip)
		}()
	}

	wg.Wait()
	close(allowed)

	count := 0
	for a := range allowed {
		if a {
			count++
		}
	}

	// Exactly 100 should be allowed (burst capacity)
	if count != 100 {
		t.Errorf("expected exactly 100 allowed, got %d", count)
	}
}

func TestGetRateLimitKey_IPFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	key, keyType := getRateLimitKey(req)

	if keyType != "ip" {
		t.Errorf("expected keyType 'ip', got %q", keyType)
	}
	if key != "ip:192.168.1.1:12345" {
		t.Errorf("expected key 'ip:192.168.1.1:12345', got %q", key)
	}
}

func TestGetRateLimitKey_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.RemoteAddr = "127.0.0.1:12345"

	key, keyType := getRateLimitKey(req)

	if keyType != "ip" {
		t.Errorf("expected keyType 'ip', got %q", keyType)
	}
	if key != "ip:203.0.113.50" {
		t.Errorf("expected key 'ip:203.0.113.50', got %q", key)
	}
}

func TestGetRateLimitKey_NightbotChannel(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
	req.Header.Set("Nightbot-Channel", "name=beastyqt&displayName=BeastyQT&provider=twitch&providerId=123")
	req.RemoteAddr = "192.168.1.1:12345"

	key, keyType := getRateLimitKey(req)

	if keyType != "channel" {
		t.Errorf("expected keyType 'channel', got %q", keyType)
	}
	if key != "channel:beastyqt" {
		t.Errorf("expected key 'channel:beastyqt', got %q", key)
	}
}

func TestGetRateLimitKey_InvalidNightbotFallsBackToIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
	req.Header.Set("Nightbot-Channel", "invalid-format")
	req.RemoteAddr = "192.168.1.1:12345"

	_, keyType := getRateLimitKey(req)

	if keyType != "ip" {
		t.Errorf("expected fallback to 'ip', got %q", keyType)
	}
}

func TestGetRateLimitKey_EmptyNightbotNameFallsBackToIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
	req.Header.Set("Nightbot-Channel", "name=&provider=twitch")
	req.RemoteAddr = "192.168.1.1:12345"

	_, keyType := getRateLimitKey(req)

	if keyType != "ip" {
		t.Errorf("expected fallback to 'ip' for empty channel name, got %q", keyType)
	}
}

func TestRateLimiterMiddleware_AllowsRequests(t *testing.T) {
	rl := newTestRateLimiter(1, time.Second, 5)

	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
	w := httptest.NewRecorder()

	rl.Middleware(handler).ServeHTTP(w, req)

	if !handlerCalled {
		t.Error("handler should have been called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRateLimiterMiddleware_BlocksExcessRequests(t *testing.T) {
	rl := newTestRateLimiter(1, time.Second, 2)

	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	})

	middleware := rl.Middleware(handler)

	// Make 5 requests from same IP
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/quote", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		if i < 2 {
			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
			}
		} else {
			if w.Code != http.StatusTooManyRequests {
				t.Errorf("request %d: expected 429, got %d", i+1, w.Code)
			}
		}
	}

	if callCount != 2 {
		t.Errorf("handler should have been called 2 times, got %d", callCount)
	}
}
