package observability_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-loop/worker/internal/adapter/observability"
)

func TestMetrics_RecordAttempt_exposesLatencyToolsTokensHallucinations(t *testing.T) {
	m := observability.NewMetrics()
	m.RecordAttempt(observability.AttemptMetrics{
		Latency:       150 * time.Millisecond,
		ToolOK:        2,
		ToolErr:       1,
		Tokens:        420,
		Hallucination: true,
	})
	m.RecordAttempt(observability.AttemptMetrics{
		Latency:       50 * time.Millisecond,
		ToolOK:        1,
		ToolErr:       0,
		Tokens:        80,
		Hallucination: false,
	})

	mux := http.NewServeMux()
	m.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	text := string(body)

	for _, want := range []string{
		"attempt_latency_ms_sum",
		"attempt_latency_ms_count 2",
		"tool_calls_total{status=\"ok\"} 3",
		"tool_calls_total{status=\"error\"} 1",
		"tokens_total 500",
		"hallucinations_total 1",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("metrics missing %q\n%s", want, text)
		}
	}
	if !strings.Contains(text, "200") { // 150+50 = 200 ms sum
		t.Errorf("expected latency sum 200 in:\n%s", text)
	}
}
