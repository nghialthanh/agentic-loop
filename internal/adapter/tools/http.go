package tools

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agentic-loop/worker/internal/service/attempt"
)

// HTTPExecutor calls tool endpoints over net/http.
// BearerToken is used when a tool declares auth "bearer" (from TOOL_BEARER_TOKEN).
type HTTPExecutor struct {
	Client      *http.Client
	BearerToken string
}

// Execute runs one tool HTTP request. TimeoutMS on the request caps the call
// (caller should already deduct from the message budget).
func (e *HTTPExecutor) Execute(ctx context.Context, req attempt.ToolHTTPRequest) attempt.ToolHTTPResponse {
	start := time.Now()
	timeout := time.Duration(req.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead && len(req.Arguments) > 0 {
		body = bytes.NewReader(req.Arguments)
	}

	httpReq, err := http.NewRequestWithContext(callCtx, method, req.Endpoint, body)
	if err != nil {
		return attempt.ToolHTTPResponse{LatencyMs: int(time.Since(start).Milliseconds()), Err: err}
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if strings.EqualFold(req.Auth, "bearer") && e.BearerToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+e.BearerToken)
	}

	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(httpReq)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return attempt.ToolHTTPResponse{LatencyMs: latency, Err: err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return attempt.ToolHTTPResponse{LatencyMs: latency, Err: err}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return attempt.ToolHTTPResponse{
			Body:      raw,
			LatencyMs: latency,
			Err:       &httpStatusError{code: res.StatusCode, body: string(raw)},
		}
	}
	return attempt.ToolHTTPResponse{Body: raw, LatencyMs: latency}
}

type httpStatusError struct {
	code int
	body string
}

func (e *httpStatusError) Error() string {
	return strings.TrimSpace(http.StatusText(e.code) + ": " + e.body)
}
