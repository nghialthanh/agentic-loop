package attempt_test

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-loop/worker/internal/attempt"
	"github.com/agentic-loop/worker/internal/contract"
)

type fakeCatalog struct{}
type fakeModel struct{}
type fakeTools struct{}
type fakeStore struct{}
type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

func TestRun_validMessageReturnsWellFormedOutput(t *testing.T) {
	fixed := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	a := attempt.New(attempt.Deps{
		Catalog: fakeCatalog{},
		Model:   fakeModel{},
		Tools:   fakeTools{},
		Store:   fakeStore{},
		Clock:   fakeClock{now: fixed},
	})

	in := contract.InputMessage{
		MessageID:      "msg_01JABC",
		ConversationID: "conv_123",
		CustomerID:     "cus_456",
		Content:        "Where is order 1001?",
		ConversationHistory: []contract.HistoryTurn{
			{Role: "customer", Content: "Hi"},
		},
	}

	result, err := a.Run(context.Background(), in)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.DeadLetter != nil {
		t.Fatalf("unexpected dead letter: %+v", result.DeadLetter)
	}
	out := result.Output
	if out == nil {
		t.Fatal("expected output")
	}
	if out.MessageID != in.MessageID {
		t.Errorf("message_id = %q", out.MessageID)
	}
	if out.ConversationID != in.ConversationID {
		t.Errorf("conversation_id = %q", out.ConversationID)
	}
	if out.Reply == "" {
		t.Error("reply must be non-empty")
	}
	if out.TraceID == "" {
		t.Error("trace_id must be set")
	}
	if out.Error != nil {
		t.Errorf("error want nil, got %+v", out.Error)
	}
	if out.Skills == nil {
		t.Error("skills must be non-nil slice")
	}
	if out.UsedTools == nil {
		t.Error("used_tools must be non-nil slice")
	}
	if out.Confidence < 0 || out.Confidence > 1 {
		t.Errorf("confidence out of range: %v", out.Confidence)
	}
}
