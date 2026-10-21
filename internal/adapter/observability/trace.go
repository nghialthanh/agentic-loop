package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type spanCtxKey struct{}

// Tracer starts one OpenTelemetry span per attempt.
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer wraps an OTel tracer.
func NewTracer(t trace.Tracer) *Tracer {
	if t == nil {
		t = trace.NewNoopTracerProvider().Tracer("noop")
	}
	return &Tracer{tracer: t}
}

// StartAttempt begins an "attempt" span tagged with correlation ids.
// The returned end function finishes the span (err non-nil marks error status).
func (t *Tracer) StartAttempt(ctx context.Context, fields AttemptFields) (context.Context, func(error)) {
	if t == nil {
		return ctx, func(error) {}
	}
	ctx, span := t.tracer.Start(ctx, "attempt",
		trace.WithAttributes(
			attribute.String("message_id", fields.MessageID),
			attribute.String("conversation_id", fields.ConversationID),
			attribute.String("trace_id", fields.TraceID),
		),
	)
	ctx = context.WithValue(ctx, spanCtxKey{}, span)
	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
}

// SetAttribute sets a string attribute on the current attempt span, with PII redaction.
func (t *Tracer) SetAttribute(ctx context.Context, key, value string) {
	span, _ := ctx.Value(spanCtxKey{}).(trace.Span)
	if span == nil {
		return
	}
	span.SetAttributes(attribute.String(key, RedactPII(value)))
}
