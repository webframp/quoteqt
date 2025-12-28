package srv

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

func TestRecordSecurityEvent_LogsWithoutSpan(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(oldLogger)

	// Call with background context (no span)
	ctx := context.Background()
	RecordSecurityEvent(ctx, "test_event",
		attribute.String("user.email", "test@example.com"),
		attribute.String("path", "/test"),
	)

	output := buf.String()

	// Verify log contains expected fields
	if !strings.Contains(output, "security.test_event") {
		t.Errorf("expected log to contain event name, got: %s", output)
	}
	if !strings.Contains(output, "test@example.com") {
		t.Errorf("expected log to contain user email, got: %s", output)
	}
	if !strings.Contains(output, "/test") {
		t.Errorf("expected log to contain path, got: %s", output)
	}
}

func TestRecordSecurityEvent_AllEventTypes(t *testing.T) {
	events := []struct {
		name  string
		attrs []attribute.KeyValue
	}{
		{
			name:  "auth_required",
			attrs: []attribute.KeyValue{attribute.String("path", "/quotes")},
		},
		{
			name: "permission_denied",
			attrs: []attribute.KeyValue{
				attribute.String("user.email", "user@example.com"),
				attribute.String("path", "/quotes"),
				attribute.String("reason", "not_channel_owner"),
			},
		},
		{
			name: "admin_required",
			attrs: []attribute.KeyValue{
				attribute.String("user.email", "user@example.com"),
				attribute.String("path", "/admin/owners"),
			},
		},
		{
			name: "rate_limited",
			attrs: []attribute.KeyValue{
				attribute.String("rate_limit.key", "ip:192.168.1.1"),
				attribute.String("rate_limit.key_type", "ip"),
				attribute.String("path", "/api/quote"),
			},
		},
	}

	for _, tt := range events {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			oldLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
			defer slog.SetDefault(oldLogger)

			ctx := context.Background()
			RecordSecurityEvent(ctx, tt.name, tt.attrs...)

			output := buf.String()
			expectedEvent := "security." + tt.name
			if !strings.Contains(output, expectedEvent) {
				t.Errorf("expected log to contain %q, got: %s", expectedEvent, output)
			}
		})
	}
}

// Integration tests for security events in handlers

func TestSecurityEvents_AuthRequired(t *testing.T) {
	server := setupTestServer(t, []string{"admin@test.com"})

	// Capture logs
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(oldLogger)

	// Request without auth headers
	req := httptest.NewRequest("GET", "/quotes", nil)
	w := httptest.NewRecorder()

	server.HandleQuotes(w, req)

	// Should redirect (303)
	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}

	// Should log auth_required event
	output := buf.String()
	if !strings.Contains(output, "security.auth_required") {
		t.Errorf("expected auth_required event, got: %s", output)
	}
}

func TestSecurityEvents_PermissionDenied(t *testing.T) {
	server := setupTestServer(t, []string{"admin@test.com"})

	// Capture logs
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(oldLogger)

	// Request with auth but no channel ownership
	req := httptest.NewRequest("GET", "/quotes", nil)
	req.Header.Set("X-ExeDev-UserID", "user123")
	req.Header.Set("X-ExeDev-Email", "nobody@example.com")
	w := httptest.NewRecorder()

	server.HandleQuotes(w, req)

	// Should return 403
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}

	// Should log permission_denied event
	output := buf.String()
	if !strings.Contains(output, "security.permission_denied") {
		t.Errorf("expected permission_denied event, got: %s", output)
	}
	if !strings.Contains(output, "nobody@example.com") {
		t.Errorf("expected user email in log, got: %s", output)
	}
}

func TestSecurityEvents_AdminRequired(t *testing.T) {
	server := setupTestServer(t, []string{"admin@test.com"})

	// Capture logs
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(oldLogger)

	// Request with auth but not admin
	req := httptest.NewRequest("GET", "/admin/owners", nil)
	req.Header.Set("X-ExeDev-UserID", "user123")
	req.Header.Set("X-ExeDev-Email", "notadmin@example.com")
	w := httptest.NewRecorder()

	server.HandleListChannelOwners(w, req)

	// Should return 403
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}

	// Should log admin_required event
	output := buf.String()
	if !strings.Contains(output, "security.admin_required") {
		t.Errorf("expected admin_required event, got: %s", output)
	}
}

func TestSecurityEvents_AdminAllowed(t *testing.T) {
	server := setupTestServer(t, []string{"admin@test.com"})

	// Capture logs
	var buf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(oldLogger)

	// Request with admin email
	req := httptest.NewRequest("GET", "/admin/owners", nil)
	req.Header.Set("X-ExeDev-UserID", "admin123")
	req.Header.Set("X-ExeDev-Email", "admin@test.com")
	w := httptest.NewRecorder()

	server.HandleListChannelOwners(w, req)

	// Should return 200 (not 403)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Should NOT log any security events
	output := buf.String()
	if strings.Contains(output, "security.") {
		t.Errorf("expected no security events for allowed access, got: %s", output)
	}
}
