package attempt_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/agentic-loop/worker/internal/attempt"
	"github.com/agentic-loop/worker/internal/catalog"
	"github.com/agentic-loop/worker/internal/observability"
	"github.com/agentic-loop/worker/internal/store"
)

func TestRun_emitsStructuredLogTraceAndMetrics(t *testing.T) {
	var logBuf bytes.Buffer
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tel := observability.NewForTest(&logBuf, tp.Tracer("test"))

	mem := store.NewMemory()
	cat := &fakeCatalog{skills: nil}
	model := &fakeModel{selectionJSON: `{"intent":"general","skill_names":[],"reason":"none"}`}
	fixed := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	a := attempt.New(attempt.Deps{
		Catalog:   cat,
		Model:     model,
		Tools:     fakeTools{},
		Store:     mem,
		Clock:     fakeClock{now: fixed},
		Telemetry: tel,
	})

	in := validInput()
	res, err := a.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Output == nil {
		t.Fatal("expected output")
	}
	if res.Output.TraceID == "" {
		t.Fatal("trace_id empty")
	}

	line := strings.TrimSpace(logBuf.String())
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("log not JSON: %v\n%s", err, line)
	}
	if rec["message_id"] != in.MessageID {
		t.Errorf("log message_id = %v", rec["message_id"])
	}
	if rec["conversation_id"] != in.ConversationID {
		t.Errorf("log conversation_id = %v", rec["conversation_id"])
	}
	if rec["trace_id"] != res.Output.TraceID {
		t.Errorf("log trace_id = %v, want %v", rec["trace_id"], res.Output.TraceID)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("otel spans = %d, want 1", len(spans))
	}

	mux := http.NewServeMux()
	tel.Metrics.Register(mux)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recHTTP := httptest.NewRecorder()
	mux.ServeHTTP(recHTTP, req)
	body, _ := io.ReadAll(recHTTP.Body)
	text := string(body)
	if !strings.Contains(text, "attempt_latency_ms_count 1") {
		t.Errorf("metrics missing attempt count:\n%s", text)
	}
	if !strings.Contains(text, "tokens_total") {
		t.Errorf("metrics missing tokens:\n%s", text)
	}
}

func TestRun_takeoverIssuesNewTraceID(t *testing.T) {
	mem := store.NewMemory()
	mem.ClaimTTL = 30 * time.Second
	clock := &mutableClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	api := &recordingOrderAPI{
		body: json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
	}
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: []attempt.ToolCallingResult{
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_1",
					Name:      "get_order_status",
					Arguments: json.RawMessage(`{"order_id":"1001"}`),
				}},
			},
			{Reply: "Order 1001 is shipped via UPS."},
		},
	}

	if _, err := mem.Claim(context.Background(), validInput().MessageID, clock.Now()); err != nil {
		t.Fatalf("seed claim: %v", err)
	}
	a := attempt.New(attempt.Deps{
		Catalog: cat,
		Model:   model,
		Tools:   api,
		Store:   mem,
		Clock:   clock,
	})
	clock.now = clock.now.Add(mem.ClaimTTL + time.Second)
	first, err := a.Run(context.Background(), validInput())
	if err != nil {
		t.Fatalf("takeover: %v", err)
	}
	trace1 := first.Output.TraceID

	// Second distinct message → different attempt/trace_id.
	in2 := validInput()
	in2.MessageID = "msg_other"
	model2 := &fakeModel{selectionJSON: `{"intent":"general","skill_names":[],"reason":"none"}`}
	a2 := attempt.New(attempt.Deps{
		Catalog: cat,
		Model:   model2,
		Tools:   api,
		Store:   mem,
		Clock:   clock,
	})
	second, err := a2.Run(context.Background(), in2)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Output.TraceID == "" || second.Output.TraceID == trace1 {
		t.Errorf("each attempt needs a distinct trace_id; got %q and %q", trace1, second.Output.TraceID)
	}
}

func TestRun_hallucinationIncrementsMetric(t *testing.T) {
	tel := observability.NewForTest(io.Discard, nil)
	mem := store.NewMemory()
	cat := &toolAwareCatalog{
		skills: []catalog.Skill{orderSupportCandidate()},
		tools:  orderSupportTools(),
	}
	// Model invents an order id that tools never returned.
	model := &scriptedModel{
		selectionJSON: `{"intent":"order_status","skill_names":["order_support"],"reason":"status"}`,
		toolResponses: []attempt.ToolCallingResult{
			{
				ToolCalls: []attempt.ToolCall{{
					ID:        "call_1",
					Name:      "get_order_status",
					Arguments: json.RawMessage(`{"order_id":"1001"}`),
				}},
			},
			{Reply: "Order 9999 was refunded yesterday."},
		},
	}
	api := &recordingOrderAPI{
		body: json.RawMessage(`{"order_id":"1001","status":"shipped","carrier":"UPS"}`),
	}
	fixed := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	a := attempt.New(attempt.Deps{
		Catalog:   cat,
		Model:     model,
		Tools:     api,
		Store:     mem,
		Clock:     fakeClock{now: fixed},
		Telemetry: tel,
	})
	in := validInput()
	in.Content = "Where is order 1001?"
	if _, err := a.Run(context.Background(), in); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mux := http.NewServeMux()
	tel.Metrics.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "hallucinations_total 1") {
		t.Fatalf("expected hallucination metric, got:\n%s", body)
	}
	if !strings.Contains(string(body), `tool_calls_total{status="ok"} 1`) {
		t.Fatalf("expected tool ok metric, got:\n%s", body)
	}
}
