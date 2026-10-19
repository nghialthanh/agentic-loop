package health_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-loop/worker/internal/health"
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
