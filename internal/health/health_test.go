package health_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-loop/worker/internal/health"
	"github.com/agentic-loop/worker/internal/observability"
)

func TestHealthEndpoint_respondsSuccessfully(t *testing.T) {
	mux := http.NewServeMux()
	health.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRegisterAll_exposesHealthAndMetrics(t *testing.T) {
	m := observability.NewMetrics()
	m.RecordAttempt(observability.AttemptMetrics{Latency: time.Millisecond, Tokens: 1})
	mux := http.NewServeMux()
	health.RegisterAll(mux, m)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "tokens_total 1") {
		t.Fatalf("metrics body: %s", body)
	}
}
