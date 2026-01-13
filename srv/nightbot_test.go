package srv

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// setupNightbotTestServer creates a test server with Nightbot import token configured
func setupNightbotTestServer(t *testing.T, importToken string, adminEmails []string) *Server {
	t.Helper()
	tempDB := filepath.Join(t.TempDir(), "test_nightbot.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname", adminEmails)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	server.Config.NightbotImportToken = importToken
	return server
}

func TestHandleNightbotImportSnapshot(t *testing.T) {
	t.Run("requires authentication", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"channel": "testchannel", "commands": [{"name": "!test", "message": "hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		// No auth headers

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("accepts admin via exe.dev headers", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"channel": "testchannel", "commands": [{"name": "!test", "message": "hello", "coolDown": 5, "userLevel": "everyone"}]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-ExeDev-Email", "admin@test.com")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if resp["success"] != true {
			t.Errorf("expected success=true, got %v", resp["success"])
		}
		if resp["channel"] != "testchannel" {
			t.Errorf("expected channel=testchannel, got %v", resp["channel"])
		}
	})

	t.Run("accepts import token", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"channel": "testchannel", "commands": [{"name": "!test", "message": "hello", "coolDown": 5, "userLevel": "everyone"}]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Import-Token", "secret-token")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}
	})

	t.Run("rejects wrong import token", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"channel": "testchannel", "commands": [{"name": "!test", "message": "hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Import-Token", "wrong-token")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("rejects non-admin with exe.dev headers", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"channel": "testchannel", "commands": [{"name": "!test", "message": "hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-ExeDev-Email", "notadmin@test.com")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("rejects GET method", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		req := httptest.NewRequest(http.MethodGet, "/admin/nightbot/import-snapshot", nil)
		req.Header.Set("X-Import-Token", "secret-token")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
		}
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString("not json"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Import-Token", "secret-token")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("rejects missing channel", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"commands": [{"name": "!test", "message": "hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Import-Token", "secret-token")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("rejects empty commands", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"channel": "testchannel", "commands": []}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Import-Token", "secret-token")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("normalizes channel name to lowercase", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"channel": "TestChannel", "commands": [{"name": "!test", "message": "hello", "coolDown": 5, "userLevel": "everyone"}]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Import-Token", "secret-token")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if resp["channel"] != "testchannel" {
			t.Errorf("expected channel=testchannel, got %v", resp["channel"])
		}
	})

	t.Run("parses exportedAt timestamp", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"channel": "testchannel", "exportedAt": "2026-01-10T12:00:00Z", "commands": [{"name": "!test", "message": "hello", "coolDown": 5, "userLevel": "everyone"}]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Import-Token", "secret-token")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}
		if resp["snapshotAt"] != "2026-01-10T12:00:00Z" {
			t.Errorf("expected snapshotAt=2026-01-10T12:00:00Z, got %v", resp["snapshotAt"])
		}
	})

	t.Run("handles invalid exportedAt gracefully", func(t *testing.T) {
		server := setupNightbotTestServer(t, "secret-token", []string{"admin@test.com"})

		body := `{"channel": "testchannel", "exportedAt": "not-a-date", "commands": [{"name": "!test", "message": "hello", "coolDown": 5, "userLevel": "everyone"}]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/nightbot/import-snapshot", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Import-Token", "secret-token")

		rr := httptest.NewRecorder()
		server.HandleNightbotImportSnapshot(rr, req)

		// Should succeed but use current time
		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d: %s", http.StatusOK, rr.Code, rr.Body.String())
		}
	})
}
