package kafka_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/agentic-loop/worker/internal/attempt"
	"github.com/agentic-loop/worker/internal/contract"
	kafkaworker "github.com/agentic-loop/worker/internal/kafka"
	"github.com/agentic-loop/worker/internal/store"
)

type fakeAttempt struct {
	result attempt.Result
	err    error
	calls  [][]byte
}

func (f *fakeAttempt) RunBytes(_ context.Context, payload []byte) (attempt.Result, error) {
	f.calls = append(f.calls, append([]byte(nil), payload...))
	return f.result, f.err
}

type published struct {
	Topic string
	Key   string
	Value []byte
}

type fakeBus struct {
	published []published
	pubErr    error
	committed []string
	commitErr error
}

func (b *fakeBus) Publish(_ context.Context, topic, key string, value []byte) error {
	if b.pubErr != nil {
		return b.pubErr
	}
	b.published = append(b.published, published{
		Topic: topic,
		Key:   key,
		Value: append([]byte(nil), value...),
	})
	return nil
}

func (b *fakeBus) Commit(_ context.Context, messageID string) error {
	if b.commitErr != nil {
		return b.commitErr
	}
	b.committed = append(b.committed, messageID)
	return nil
}

func TestHandle_refusesInvalidOutputWithoutPublish(t *testing.T) {
	out := &contract.OutputMessage{
		MessageID:      "",
		ConversationID: "conv_1",
		Reply:          "bad",
		Skills:         []string{},
		UsedTools:      []contract.UsedTool{},
		TraceID:        "trace_1",
	}
	runner := &fakeAttempt{result: attempt.Result{Output: out}}
	bus := &fakeBus{}
	w := kafkaworker.NewWorker(runner, bus)
	err := w.Handle(context.Background(), kafkaworker.Record{Key: "msg", Payload: []byte(`{}`)})
	if err == nil {
		t.Fatal("expected refuse invalid output error")
	}
	if len(bus.published) != 0 {
		t.Fatalf("published = %d, want 0", len(bus.published))
	}
	if len(bus.committed) != 0 {
		t.Fatalf("committed = %d, want 0", len(bus.committed))
	}
}

func TestHandle_happyPathPublishesOutputThenCommits(t *testing.T) {
	out := &contract.OutputMessage{
		MessageID:      "msg_happy",
		ConversationID: "conv_1",
		Reply:          "Order is shipped.",
		Skills:         []string{},
		UsedTools:      []contract.UsedTool{},
		TraceID:        "trace_1",
	}
	runner := &fakeAttempt{result: attempt.Result{Output: out}}
	bus := &fakeBus{}
	w := kafkaworker.NewWorker(runner, bus)

	payload := []byte(`{"message_id":"msg_happy","conversation_id":"conv_1","customer_id":"cus_1","content":"hi","conversation_history":[]}`)
	if err := w.Handle(context.Background(), kafkaworker.Record{
		Key:     "msg_happy",
		Payload: payload,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("attempt calls = %d, want 1", len(runner.calls))
	}
	if len(bus.published) != 1 {
		t.Fatalf("publishes = %d, want 1", len(bus.published))
	}
	p := bus.published[0]
	if p.Topic != kafkaworker.TopicOutput {
		t.Errorf("topic = %q, want %q", p.Topic, kafkaworker.TopicOutput)
	}
	if p.Key != "msg_happy" {
		t.Errorf("key = %q, want message_id", p.Key)
	}
	var got contract.OutputMessage
	if err := json.Unmarshal(p.Value, &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got.Reply != out.Reply {
		t.Errorf("reply = %q, want %q", got.Reply, out.Reply)
	}
	if len(bus.committed) != 1 || bus.committed[0] != "msg_happy" {
		t.Fatalf("committed = %v, want [msg_happy]", bus.committed)
	}
}

func TestHandle_malformedPublishesDLQThenCommits(t *testing.T) {
	dl := &contract.DeadLetter{
		Payload: json.RawMessage(`{"message_id":123}`),
		Reason:  "schema validation failed",
	}
	runner := &fakeAttempt{result: attempt.Result{DeadLetter: dl}}
	bus := &fakeBus{}
	w := kafkaworker.NewWorker(runner, bus)

	payload := []byte(`{"message_id":123}`)
	if err := w.Handle(context.Background(), kafkaworker.Record{
		Key:     "msg_bad",
		Payload: payload,
	}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(bus.published) != 1 {
		t.Fatalf("publishes = %d, want 1", len(bus.published))
	}
	p := bus.published[0]
	if p.Topic != kafkaworker.TopicDLQ {
		t.Errorf("topic = %q, want %q", p.Topic, kafkaworker.TopicDLQ)
	}
	if p.Key != "msg_bad" {
		t.Errorf("dlq key = %q, want inbound message_id key", p.Key)
	}
	var got contract.DeadLetter
	if err := json.Unmarshal(p.Value, &got); err != nil {
		t.Fatalf("unmarshal dlq: %v", err)
	}
	if got.Reason != dl.Reason {
		t.Errorf("reason = %q, want %q", got.Reason, dl.Reason)
	}
	if len(bus.committed) != 1 || bus.committed[0] != "msg_bad" {
		t.Fatalf("committed = %v, want [msg_bad]", bus.committed)
	}
}

func TestHandle_publishFailureDoesNotCommit(t *testing.T) {
	out := &contract.OutputMessage{
		MessageID:      "msg_pubfail",
		ConversationID: "conv_1",
		Reply:          "ok",
		Skills:         []string{},
		UsedTools:      []contract.UsedTool{},
		TraceID:        "t",
	}
	runner := &fakeAttempt{result: attempt.Result{Output: out}}
	bus := &fakeBus{pubErr: errors.New("broker down")}
	w := kafkaworker.NewWorker(runner, bus)

	err := w.Handle(context.Background(), kafkaworker.Record{
		Key:     "msg_pubfail",
		Payload: []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected publish error")
	}
	if len(bus.committed) != 0 {
		t.Fatalf("must not commit after publish failure, committed=%v", bus.committed)
	}
}

func TestHandle_claimHeldDoesNotPublishOrCommit(t *testing.T) {
	runner := &fakeAttempt{err: store.ErrClaimHeld}
	bus := &fakeBus{}
	w := kafkaworker.NewWorker(runner, bus)

	err := w.Handle(context.Background(), kafkaworker.Record{
		Key:     "msg_held",
		Payload: []byte(`{}`),
	})
	if !errors.Is(err, store.ErrClaimHeld) {
		t.Fatalf("err = %v, want ErrClaimHeld", err)
	}
	if len(bus.published) != 0 {
		t.Fatalf("published = %d, want 0", len(bus.published))
	}
	if len(bus.committed) != 0 {
		t.Fatalf("committed = %v, want none", bus.committed)
	}
}
