package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agentic-loop/worker/internal/attempt"
	"github.com/agentic-loop/worker/internal/contract"
)

// Runner runs one inbound Kafka payload through the attempt seam.
type Runner interface {
	RunBytes(ctx context.Context, payload []byte) (attempt.Result, error)
}

// Bus publishes replies/DLQ records and commits the input offset after success.
// Commit is keyed by the inbound record's message_id (Kafka key) for testability;
// the real adapter commits the fetched Kafka message.
type Bus interface {
	Publish(ctx context.Context, topic, key string, value []byte) error
	Commit(ctx context.Context, messageID string) error
}

// Record is one consumed input message.
type Record struct {
	Key     string
	Payload []byte
}

// Worker consumes one record: attempt → publish → commit offset.
type Worker struct {
	runner Runner
	bus    Bus
}

// NewWorker builds a Worker over the attempt seam and Kafka bus.
func NewWorker(runner Runner, bus Bus) *Worker {
	return &Worker{runner: runner, bus: bus}
}

// Handle runs store-then-publish-then-commit for one input record.
// On publish failure the offset is not committed so the message can be redelivered.
// ClaimHeld and other attempt errors also skip commit.
func (w *Worker) Handle(ctx context.Context, rec Record) error {
	result, err := w.runner.RunBytes(ctx, rec.Payload)
	if err != nil {
		return err
	}

	key := rec.Key
	var topic string
	var value []byte

	switch {
	case result.DeadLetter != nil:
		topic = TopicDLQ
		value, err = json.Marshal(result.DeadLetter)
		if err != nil {
			return fmt.Errorf("marshal dead letter: %w", err)
		}
		if key == "" {
			key = messageIDFromPayload(result.DeadLetter.Payload)
		}
	case result.Output != nil:
		if err := contract.ValidateOutput(*result.Output); err != nil {
			return fmt.Errorf("refuse invalid output: %w", err)
		}
		topic = TopicOutput
		key = result.Output.MessageID
		value, err = json.Marshal(result.Output)
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
	default:
		return fmt.Errorf("attempt returned neither output nor dead letter")
	}

	if err := w.bus.Publish(ctx, topic, key, value); err != nil {
		return fmt.Errorf("publish %s: %w", topic, err)
	}
	if err := w.bus.Commit(ctx, rec.Key); err != nil {
		return fmt.Errorf("commit offset: %w", err)
	}
	return nil
}

func messageIDFromPayload(payload any) string {
	switch p := payload.(type) {
	case map[string]any:
		if id, ok := p["message_id"].(string); ok {
			return id
		}
	case json.RawMessage:
		var m map[string]any
		if json.Unmarshal(p, &m) == nil {
			if id, ok := m["message_id"].(string); ok {
				return id
			}
		}
	}
	return ""
}
