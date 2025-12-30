package srv

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// gzipResponseWriter wraps http.ResponseWriter to provide gzip compression
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// gzip writer pool to reduce allocations
var gzipPool = sync.Pool{
	New: func() any {
		gz, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return gz
	},
}

// Gzip middleware compresses responses for clients that accept it
func Gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Get gzip writer from pool
		gz := gzipPool.Get().(*gzip.Writer)
		gz.Reset(w)
		defer func() {
			gz.Close()
			gzipPool.Put(gz)
		}()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // Length changes with compression

		next.ServeHTTP(gzipResponseWriter{Writer: gz, ResponseWriter: w}, r)
	})
}

// StaticFileServer returns a handler for static files with cache headers
func StaticFileServer(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set cache headers for static assets (1 year for immutable assets)
		// CSS and JS files can be cached aggressively
		path := r.URL.Path
		if strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".js") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpg") || 
		          strings.HasSuffix(path, ".gif") || strings.HasSuffix(path, ".ico") ||
		          strings.HasSuffix(path, ".woff") || strings.HasSuffix(path, ".woff2") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Default: cache for 1 hour
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		fs.ServeHTTP(w, r)
	})
}

// responseRecorder wraps http.ResponseWriter to capture status code
type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// RequestLogger logs slow requests (>500ms) and errors
// Skips /health and /static/* paths to reduce noise
func RequestLogger(next http.Handler) http.Handler {
	const slowThreshold = 500 * time.Millisecond

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Skip noisy endpoints
		if path == "/health" || strings.HasPrefix(path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w}

		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		status := rec.status

		// Log errors (4xx, 5xx) or slow requests
		if status >= 400 || duration > slowThreshold {
			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
			} else if status >= 400 {
				level = slog.LevelWarn
			}

			slog.Log(r.Context(), level, "request",
				"method", r.Method,
				"path", path,
				"status", status,
				"duration", duration.Round(time.Millisecond),
			)
		}
	})
}

// SecurityHeaders adds security-related HTTP headers to responses
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")
		
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")
		
		// Control referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		
		// Content Security Policy
		// - default-src 'self': Only allow resources from same origin by default
		// - script-src: Allow self, unpkg.com for Lucide, and unsafe-inline for theme toggle etc.
		// - style-src: Allow self, inline styles (for theme.css vars), and Google Fonts
		// - font-src: Allow Google Fonts
		// - img-src: Allow self and data URIs (for inline images)
		// - connect-src: Allow self for API calls
		// Note: 'unsafe-inline' in script-src is needed for onclick handlers and inline scripts.
		// In a future iteration, these could be moved to external scripts with nonces.
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' https://unpkg.com https://cdn.jsdelivr.net; " +
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://unpkg.com https://cdn.jsdelivr.net; " +
			"font-src https://fonts.gstatic.com https://cdn.jsdelivr.net; " +
			"img-src 'self' data: https://cdn.jsdelivr.net; " +
			"connect-src 'self' https://proxy.scalar.com"
		w.Header().Set("Content-Security-Policy", csp)
		
		next.ServeHTTP(w, r)
	})
}
