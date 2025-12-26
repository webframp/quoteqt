package srv

import (
	"context"
	"runtime"

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
