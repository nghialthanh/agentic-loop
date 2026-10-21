package observability

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// AttemptMetrics is one completed attempt's recorded counters.
type AttemptMetrics struct {
	Latency       time.Duration
	ToolOK        int
	ToolErr       int
	Tokens        int
	Hallucination bool
}

// Metrics accumulates worker-level attempt metrics for the /metrics endpoint.
type Metrics struct {
	latencySumMs   atomic.Int64
	latencyCount   atomic.Int64
	toolOK         atomic.Int64
	toolErr        atomic.Int64
	tokens         atomic.Int64
	hallucinations atomic.Int64
}

// NewMetrics returns an empty metrics registry.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordAttempt adds one attempt's latency, tool outcomes, tokens, and hallucination flag.
func (m *Metrics) RecordAttempt(a AttemptMetrics) {
	if m == nil {
		return
	}
	m.latencySumMs.Add(a.Latency.Milliseconds())
	m.latencyCount.Add(1)
	m.toolOK.Add(int64(a.ToolOK))
	m.toolErr.Add(int64(a.ToolErr))
	m.tokens.Add(int64(a.Tokens))
	if a.Hallucination {
		m.hallucinations.Add(1)
	}
}

// Register mounts GET /metrics on mux.
func (m *Metrics) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = fmt.Fprintf(w, "# HELP attempt_latency_ms End-to-end attempt latency in milliseconds\n")
		_, _ = fmt.Fprintf(w, "# TYPE attempt_latency_ms summary\n")
		_, _ = fmt.Fprintf(w, "attempt_latency_ms_sum %d\n", m.latencySumMs.Load())
		_, _ = fmt.Fprintf(w, "attempt_latency_ms_count %d\n", m.latencyCount.Load())
		_, _ = fmt.Fprintf(w, "# HELP tool_calls_total Tool HTTP call outcomes\n")
		_, _ = fmt.Fprintf(w, "# TYPE tool_calls_total counter\n")
		_, _ = fmt.Fprintf(w, "tool_calls_total{status=\"ok\"} %d\n", m.toolOK.Load())
		_, _ = fmt.Fprintf(w, "tool_calls_total{status=\"error\"} %d\n", m.toolErr.Load())
		_, _ = fmt.Fprintf(w, "# HELP tokens_total Estimated model token usage\n")
		_, _ = fmt.Fprintf(w, "# TYPE tokens_total counter\n")
		_, _ = fmt.Fprintf(w, "tokens_total %d\n", m.tokens.Load())
		_, _ = fmt.Fprintf(w, "# HELP hallucinations_total Attempts where the reply asserted unsupported account facts\n")
		_, _ = fmt.Fprintf(w, "# TYPE hallucinations_total counter\n")
		_, _ = fmt.Fprintf(w, "hallucinations_total %d\n", m.hallucinations.Load())
	})
}
