package health

import (
	"net/http"

	"github.com/agentic-loop/worker/internal/adapter/observability"
)

// Register mounts the health endpoint on mux.
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

// RegisterAll mounts /health and /metrics (when metrics is non-nil).
func RegisterAll(mux *http.ServeMux, metrics *observability.Metrics) {
	Register(mux)
	if metrics != nil {
		metrics.Register(mux)
	}
}
