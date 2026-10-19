package attempt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/agentic-loop/worker/internal/contract"
)

// Catalog looks up skills and tools. Ticket 01 uses a stub behind this seam.
type Catalog interface{}

// Model calls the language model. Ticket 01 uses a stub behind this seam.
type Model interface{}

// Tools calls tool HTTP APIs. Ticket 01 uses a stub behind this seam.
type Tools interface{}

// Store holds dedupe, claims, and rate limits. Ticket 01 uses a stub behind this seam.
type Store interface{}

// Clock provides the current time for attempts.
type Clock interface {
	Now() time.Time
}

// Deps are adapters the attempt seam needs.
type Deps struct {
	Catalog Catalog
	Model   Model
	Tools   Tools
	Store   Store
	Clock   Clock
}

// Attempt is the primary seam: one inbound message in; one output or dead-letter out.
type Attempt struct {
	deps Deps
}

// New builds an Attempt with the given adapters.
func New(deps Deps) *Attempt {
	return &Attempt{deps: deps}
}

// Result is either a normal output or a dead-letter record.
type Result struct {
	Output     *contract.OutputMessage
	DeadLetter *contract.DeadLetter
}

// Run processes one message through the attempt seam.
// Ticket 01 returns a stub reply for valid input; full pipeline comes later.
func (a *Attempt) Run(ctx context.Context, in contract.InputMessage) (Result, error) {
	_ = ctx
	_ = a.deps

	if reason := validateInput(in); reason != "" {
		return Result{
			DeadLetter: &contract.DeadLetter{Payload: in, Reason: reason},
		}, nil
	}

	traceID, err := newTraceID()
	if err != nil {
		return Result{}, fmt.Errorf("trace id: %w", err)
	}

	return Result{
		Output: &contract.OutputMessage{
			MessageID:      in.MessageID,
			ConversationID: in.ConversationID,
			Reply:          "Thanks for your message. I'm looking into it.",
			Intent:         "",
			Skills:         []string{},
			UsedTools:      []contract.UsedTool{},
			Confidence:     0.2,
			Error:          nil,
			TraceID:        traceID,
			Model:          "",
		},
	}, nil
}

func validateInput(in contract.InputMessage) string {
	if strings.TrimSpace(in.MessageID) == "" {
		return "message_id is required"
	}
	if strings.TrimSpace(in.ConversationID) == "" {
		return "conversation_id is required"
	}
	if strings.TrimSpace(in.CustomerID) == "" {
		return "customer_id is required"
	}
	if strings.TrimSpace(in.Content) == "" {
		return "content is empty after trim"
	}
	if in.ConversationHistory == nil {
		return "conversation_history is required"
	}
	for i, turn := range in.ConversationHistory {
		if turn.Role != "customer" && turn.Role != "agent" {
			return fmt.Sprintf("conversation_history[%d]: unknown role %q", i, turn.Role)
		}
	}
	return ""
}

func newTraceID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
