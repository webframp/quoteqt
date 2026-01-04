package srv

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	// Simple handler that just returns OK
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := SecurityHeaders(inner)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Check all security headers are set
	tests := []struct {
		header string
		want   string
	}{
		{"X-Frame-Options", "DENY"},
		{"X-Content-Type-Options", "nosniff"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	}

	for _, tt := range tests {
		got := rec.Header().Get(tt.header)
		if got != tt.want {
			t.Errorf("%s = %q, want %q", tt.header, got, tt.want)
		}
	}

	// CSP should be set and contain key directives
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header not set")
	}
	if !strings.Contains(csp, "default-src 'self'") {
		t.Error("CSP missing default-src 'self'")
	}
}

func TestGzip_WithAcceptEncoding(t *testing.T) {
	// Handler that returns a known response
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	})

	handler := Gzip(inner)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should have gzip encoding header
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Error("Content-Encoding should be gzip")
	}

	// Content-Length should be removed
	if rec.Header().Get("Content-Length") != "" {
		t.Error("Content-Length should be removed for gzipped response")
	}

	// Body should be valid gzip
	gr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("response is not valid gzip: %v", err)
	}
	defer gr.Close()

	body, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to read gzipped body: %v", err)
	}

	if string(body) != "hello world" {
		t.Errorf("got %q, want %q", string(body), "hello world")
	}
}

func TestGzip_WithoutAcceptEncoding(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world"))
	})

	handler := Gzip(inner)

	req := httptest.NewRequest("GET", "/", nil)
	// No Accept-Encoding header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should NOT have gzip encoding
	if rec.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not gzip without Accept-Encoding")
	}

	// Body should be plain text
	if rec.Body.String() != "hello world" {
		t.Errorf("got %q, want %q", rec.Body.String(), "hello world")
	}
}

func TestRequestLogger_SkipsHealth(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestLogger(inner)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestRequestLogger_SkipsStatic(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := RequestLogger(inner)

	req := httptest.NewRequest("GET", "/static/style.css", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("inner handler was not called")
	}
}

func TestRequestLogger_CapturesStatus(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	handler := RequestLogger(inner)

	req := httptest.NewRequest("GET", "/notfound", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStaticFileServer_CacheHeaders(t *testing.T) {
	// Create temp directory with test files
	tmpDir, err := os.MkdirTemp("", "static-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	files := map[string]string{
		"style.css": "body {}",
		"app.js":    "console.log('hi')",
		"logo.png":  "fake png",
		"data.json": "{}",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	handler := StaticFileServer(tmpDir)

	tests := []struct {
		path      string
		wantCache string
	}{
		{"/style.css", "public, max-age=31536000, immutable"},
		{"/app.js", "public, max-age=31536000, immutable"},
		{"/logo.png", "public, max-age=31536000, immutable"},
		{"/data.json", "public, max-age=3600"}, // default
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.path, nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		got := rec.Header().Get("Cache-Control")
		if got != tt.wantCache {
			t.Errorf("%s: Cache-Control = %q, want %q", tt.path, got, tt.wantCache)
		}
	}
}

func TestResponseRecorder_DefaultStatus(t *testing.T) {
	// Test that responseRecorder defaults to 200 when Write is called without WriteHeader
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello")) // No explicit WriteHeader
	})

	handler := RequestLogger(inner)

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should default to 200
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
