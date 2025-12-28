package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("quoteqt")

// StartDBSpan starts a child span for a database operation
func StartDBSpan(ctx context.Context, operation string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("db.system", "sqlite"),
		attribute.String("db.operation", operation),
	}
	attrs = append(baseAttrs, attrs...)
	return tracer.Start(ctx, "db."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
}

// RecordSecurityEvent records a security-related event on the current span.
// Events are prefixed with "security." and also logged via slog for local visibility.
// Use this for permission denied, auth required, rate limiting, etc.
func RecordSecurityEvent(ctx context.Context, event string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		// Still log locally even if tracing is disabled
		logSecurityEvent(event, attrs)
		return
	}

	fullEvent := "security." + event
	span.AddEvent(fullEvent, trace.WithAttributes(attrs...))

	// Also log locally for visibility without Honeycomb
	logSecurityEvent(event, attrs)
}

// logSecurityEvent logs a security event to slog with structured attributes
func logSecurityEvent(event string, attrs []attribute.KeyValue) {
	args := make([]any, 0, len(attrs)*2+2)
	args = append(args, "event", "security."+event)
	for _, attr := range attrs {
		args = append(args, string(attr.Key), attr.Value.AsInterface())
	}
	slog.Warn("security event", args...)
}

// RecordError records an error on the span following OTel exception conventions.
// It adds an "exception" event with message, type, and stacktrace attributes,
// and sets the span status to Error.
func RecordError(span trace.Span, err error) {
	if err == nil || !span.IsRecording() {
		return
	}

	// Capture stack trace
	const maxStackSize = 4096
	stackBuf := make([]byte, maxStackSize)
	stackSize := runtime.Stack(stackBuf, false)
	stacktrace := string(stackBuf[:stackSize])

	// Record exception event per OTel spec
	span.AddEvent("exception",
		trace.WithAttributes(
			attribute.String("exception.type", "error"),
			attribute.String("exception.message", err.Error()),
			attribute.String("exception.stacktrace", stacktrace),
		),
	)

	// Set span status to error
	span.SetStatus(codes.Error, err.Error())
}

// WantsJSON checks if the client prefers JSON response based on Accept header.
// Returns false (plain text) by default for Nightbot compatibility.
func WantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	// Check for explicit JSON preference
	// Be lenient: accept application/json anywhere in the header
	return strings.Contains(accept, "application/json")
}

// WriteQuoteResponse writes a quote as either JSON or plain text based on Accept header.
func WriteQuoteResponse(w http.ResponseWriter, r *http.Request, quote QuoteResponse) {
	if WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(quote)
		return
	}

	// Plain text format for Nightbot compatibility
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var parts []string
	parts = append(parts, quote.Text)
	if quote.Author != nil && *quote.Author != "" {
		parts = append(parts, fmt.Sprintf("— %s", *quote.Author))
	}
	if quote.Civilization != nil && *quote.Civilization != "" {
		parts = append(parts, fmt.Sprintf("[%s]", *quote.Civilization))
	}
	fmt.Fprintln(w, strings.Join(parts, " "))
}

// WriteNoResultsResponse writes a "no results" message as either JSON or plain text.
func WriteNoResultsResponse(w http.ResponseWriter, r *http.Request, message string) {
	if WantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": message})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, message)
}
