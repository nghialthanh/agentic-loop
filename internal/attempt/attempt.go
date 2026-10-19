package attempt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

// Store holds dedupe, claims, and rate limits.
// ClaimMessage is the only operation this ticket asserts; full claim semantics arrive later.
type Store interface {
	ClaimMessage(ctx context.Context, messageID string) error
}

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

// Run processes one typed message through the attempt seam.
// Malformed input returns a dead letter and never claims the message.
func (a *Attempt) Run(ctx context.Context, in contract.InputMessage) (Result, error) {
	if reason := validateInput(in); reason != "" {
		return Result{
			DeadLetter: &contract.DeadLetter{Payload: in, Reason: reason},
		}, nil
	}
	return a.runValid(ctx, in)
}

// RunBytes unmarshals a Kafka JSON payload and runs the attempt.
// Schema-invalid JSON becomes a dead letter with the original payload; no claim is written.
func (a *Attempt) RunBytes(ctx context.Context, payload []byte) (Result, error) {
	var in contract.InputMessage
	if err := json.Unmarshal(payload, &in); err != nil {
		return Result{
			DeadLetter: &contract.DeadLetter{
				Payload: json.RawMessage(append([]byte(nil), payload...)),
				Reason:  "schema validation failed: " + err.Error(),
			},
		}, nil
	}
	if reason := validateInput(in); reason != "" {
		return Result{
			DeadLetter: &contract.DeadLetter{
				Payload: json.RawMessage(append([]byte(nil), payload...)),
				Reason:  reason,
			},
		}, nil
	}
	return a.runValid(ctx, in)
}

func (a *Attempt) runValid(ctx context.Context, in contract.InputMessage) (Result, error) {
	_ = a.deps.Catalog
	_ = a.deps.Model
	_ = a.deps.Tools
	_ = a.deps.Clock

	if err := a.deps.Store.ClaimMessage(ctx, in.MessageID); err != nil {
		return Result{}, fmt.Errorf("claim message: %w", err)
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
