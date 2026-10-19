package observability_test

import (
	"context"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/agentic-loop/worker/internal/observability"
)

func TestTracer_StartAttempt_createsSpanWithIDsAndRedactsPII(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tr := observability.NewTracer(tp.Tracer("test"))

	ctx, end := tr.StartAttempt(context.Background(), observability.AttemptFields{
		MessageID:      "msg_1",
		ConversationID: "conv_1",
		TraceID:        "deadbeefdeadbeefdeadbeefdeadbeef",
	})
	tr.SetAttribute(ctx, "note", "email me at leak@example.com")
	end(nil)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	sp := spans[0]
	if sp.Name != "attempt" {
		t.Errorf("span name = %q", sp.Name)
	}
	attrs := map[string]string{}
	for _, a := range sp.Attributes {
		attrs[string(a.Key)] = a.Value.AsString()
	}
	if attrs["message_id"] != "msg_1" {
		t.Errorf("message_id attr = %q", attrs["message_id"])
	}
	if attrs["conversation_id"] != "conv_1" {
		t.Errorf("conversation_id = %q", attrs["conversation_id"])
	}
	if attrs["trace_id"] != "deadbeefdeadbeefdeadbeefdeadbeef" {
		t.Errorf("trace_id attr = %q", attrs["trace_id"])
	}
	if strings.Contains(attrs["note"], "leak@example.com") {
		t.Errorf("PII leaked on span attr: %q", attrs["note"])
	}
}
