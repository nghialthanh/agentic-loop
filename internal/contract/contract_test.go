package contract_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-loop/worker/internal/contract"
)

func TestInputMessage_unmarshalsFoundationShape(t *testing.T) {
	raw := []byte(`{
  "message_id": "msg_01J...",
  "conversation_id": "conv_123",
  "customer_id": "cus_456",
  "content": "I was charged twice. Where is order 1001?",
  "conversation_history": [
    {
      "role": "customer",
      "content": "Hi, I need help with an order.",
      "at": "2026-10-19T03:00:00Z"
    },
    {
      "role": "agent",
      "content": "I can help with that.",
      "at": "2026-10-19T03:00:05Z"
    }
  ]
}`)

	var in contract.InputMessage
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("unmarshal input: %v", err)
	}

	if in.MessageID != "msg_01J..." {
		t.Errorf("message_id = %q", in.MessageID)
	}
	if in.ConversationID != "conv_123" {
		t.Errorf("conversation_id = %q", in.ConversationID)
	}
	if in.CustomerID != "cus_456" {
		t.Errorf("customer_id = %q", in.CustomerID)
	}
	if in.Content != "I was charged twice. Where is order 1001?" {
		t.Errorf("content = %q", in.Content)
	}
	if len(in.ConversationHistory) != 2 {
		t.Fatalf("history len = %d", len(in.ConversationHistory))
	}
	if in.ConversationHistory[0].Role != "customer" {
		t.Errorf("history[0].role = %q", in.ConversationHistory[0].Role)
	}
	if in.ConversationHistory[1].Role != "agent" {
		t.Errorf("history[1].role = %q", in.ConversationHistory[1].Role)
	}
	wantAt := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	if in.ConversationHistory[0].At == nil || !in.ConversationHistory[0].At.Equal(wantAt) {
		t.Errorf("history[0].at = %v", in.ConversationHistory[0].At)
	}
}

func TestOutputMessage_marshalsFoundationShape(t *testing.T) {
	out := contract.OutputMessage{
		MessageID:      "msg_01J...",
		ConversationID: "conv_123",
		Reply:          "Order 1001 is in transit.",
		Intent:         "order_status_and_refund",
		Skills:         []string{"order_support"},
		UsedTools: []contract.UsedTool{
			{Name: "get_order_status", Status: "ok", LatencyMs: 120},
		},
		Confidence: 0.8,
		Error:      nil,
		TraceID:    "4bf92f3577b34da6a3ce929d0e0e4736",
		Model:      "gpt-4.1",
	}

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}

	required := []string{
		"message_id", "conversation_id", "reply", "intent", "skills",
		"used_tools", "confidence", "error", "trace_id", "model",
	}
	for _, k := range required {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	if got["error"] != nil {
		t.Errorf("error want null, got %v", got["error"])
	}
	if got["message_id"] != "msg_01J..." {
		t.Errorf("message_id = %v", got["message_id"])
	}
	if got["reply"] != "Order 1001 is in transit." {
		t.Errorf("reply = %v", got["reply"])
	}

	tools, ok := got["used_tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("used_tools = %v", got["used_tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("used_tools[0] = %v", tools[0])
	}
	for _, k := range []string{"name", "status", "latency_ms"} {
		if _, ok := tool[k]; !ok {
			t.Errorf("used_tools[0] missing %q", k)
		}
	}
}

func TestOutputMessage_errorObjectShape(t *testing.T) {
	out := contract.OutputMessage{
		MessageID:      "msg_01J...",
		ConversationID: "conv_123",
		Reply:          "I'm getting too many messages right now. Please try again in a minute.",
		Intent:         "",
		Skills:         []string{},
		UsedTools:      []contract.UsedTool{},
		Confidence:     0,
		Error:          &contract.OutputError{Code: "rate_limited", Message: "customer rate limit exceeded"},
		TraceID:        "trace-1",
		Model:          "",
	}

	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj, ok := got["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %v", got["error"])
	}
	if errObj["code"] != "rate_limited" {
		t.Errorf("error.code = %v", errObj["code"])
	}
	if errObj["message"] != "customer rate limit exceeded" {
		t.Errorf("error.message = %v", errObj["message"])
	}
}
