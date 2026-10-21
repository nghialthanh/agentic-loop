package observability

import (
	"context"
	"io"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Telemetry bundles structured logs, metrics, and an OTel attempt tracer.
type Telemetry struct {
	Log     *Logger
	Metrics *Metrics
	Tracer  *Tracer
}

// NewDefault builds process-wide telemetry writing logs to stderr and using the
// global OTel tracer provider (noop until an exporter is registered).
func NewDefault() *Telemetry {
	return &Telemetry{
		Log:     NewLogger(os.Stderr),
		Metrics: NewMetrics(),
		Tracer:  NewTracer(otel.Tracer("github.com/agentic-loop/worker")),
	}
}

// NewForTest builds telemetry with a custom log sink and tracer.
func NewForTest(logOut io.Writer, tracer trace.Tracer) *Telemetry {
	return &Telemetry{
		Log:     NewLogger(logOut),
		Metrics: NewMetrics(),
		Tracer:  NewTracer(tracer),
	}
}

// BeginAttempt starts an OTel span and returns fields for logging.
func (t *Telemetry) BeginAttempt(ctx context.Context, messageID, conversationID, traceID string) (context.Context, AttemptFields, func(error)) {
	fields := AttemptFields{
		MessageID:      messageID,
		ConversationID: conversationID,
		TraceID:        traceID,
	}
	if t == nil || t.Tracer == nil {
		return ctx, fields, func(error) {}
	}
	ctx, end := t.Tracer.StartAttempt(ctx, fields)
	return ctx, fields, end
}

// FinishAttempt logs, records metrics, and ends the span.
func (t *Telemetry) FinishAttempt(fields AttemptFields, endSpan func(error), err error, m AttemptMetrics) {
	if t == nil {
		if endSpan != nil {
			endSpan(err)
		}
		return
	}
	if t.Log != nil {
		kv := []any{
			"latency_ms", m.Latency.Milliseconds(),
			"tokens", m.Tokens,
			"tool_ok", m.ToolOK,
			"tool_err", m.ToolErr,
			"hallucination", m.Hallucination,
		}
		if err != nil {
			kv = append(kv, "error", err.Error())
		}
		t.Log.AttemptInfo("attempt finished", fields, kv...)
	}
	if t.Metrics != nil {
		t.Metrics.RecordAttempt(m)
	}
	if endSpan != nil {
		endSpan(err)
	}
}

