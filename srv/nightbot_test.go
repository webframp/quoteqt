package srv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNightbotAPICallRetry(t *testing.T) {
	t.Run("succeeds on first try", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok": true}`))
		}))
		defer server.Close()

		req, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
		resp, err := nightbotAPICall(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("retries on 429", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			call := atomic.AddInt32(&calls, 1)
			if call < 2 {
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error": "rate limited"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok": true}`))
		}))
		defer server.Close()

		req, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
		resp, err := nightbotAPICall(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		if calls != 2 {
			t.Errorf("expected 2 calls, got %d", calls)
		}
	})

	t.Run("retries on 5xx", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			call := atomic.AddInt32(&calls, 1)
			if call < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"error": "service unavailable"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok": true}`))
		}))
		defer server.Close()

		req, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
		resp, err := nightbotAPICall(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		if calls != 3 {
			t.Errorf("expected 3 calls, got %d", calls)
		}
	})

	t.Run("does not retry 4xx except 429", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "bad request"}`))
		}))
		defer server.Close()

		req, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
		resp, err := nightbotAPICall(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", resp.StatusCode)
		}
		if calls != 1 {
			t.Errorf("expected 1 call, got %d", calls)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		var calls int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "rate limited"}`))
		}))
		defer server.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
		_, err := nightbotAPICall(ctx, req)
		if err == nil {
			t.Fatal("expected error due to context cancellation")
		}
		// Should have attempted at least once before timeout
		if calls < 1 {
			t.Errorf("expected at least 1 call, got %d", calls)
		}
	})
}

func TestNightbotHTTPClientTimeout(t *testing.T) {
	// Verify the client has a timeout configured
	if nightbotHTTPClient.Timeout == 0 {
		t.Error("nightbotHTTPClient should have a timeout configured")
	}
	if nightbotHTTPClient.Timeout != nightbotAPITimeout {
		t.Errorf("expected timeout %v, got %v", nightbotAPITimeout, nightbotHTTPClient.Timeout)
	}
}
