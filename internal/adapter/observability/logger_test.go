package observability_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentic-loop/worker/internal/adapter/observability"
)

func TestLogger_AttemptInfo_includesIDsAndRedactsPII(t *testing.T) {
	var buf bytes.Buffer
	log := observability.NewLogger(&buf)
	log.AttemptInfo("attempt finished", observability.AttemptFields{
		MessageID:      "msg_1",
		ConversationID: "conv_1",
		TraceID:        "trace_abc",
	}, "email", "secret@example.com")

	line := strings.TrimSpace(buf.String())
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("log line not JSON: %v\n%s", err, line)
	}
	if rec["message_id"] != "msg_1" {
		t.Errorf("message_id = %v", rec["message_id"])
	}
	if rec["conversation_id"] != "conv_1" {
		t.Errorf("conversation_id = %v", rec["conversation_id"])
	}
	if rec["trace_id"] != "trace_abc" {
		t.Errorf("trace_id = %v", rec["trace_id"])
	}
	if rec["msg"] != "attempt finished" {
		t.Errorf("msg = %v", rec["msg"])
	}
	email, _ := rec["email"].(string)
	if strings.Contains(email, "secret@example.com") {
		t.Errorf("PII leaked in log: %q", email)
	}
}
