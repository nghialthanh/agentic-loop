package kafka_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-loop/worker/internal/attempt"
	"github.com/agentic-loop/worker/internal/catalog"
	"github.com/agentic-loop/worker/internal/contract"
	kafkaworker "github.com/agentic-loop/worker/internal/kafka"
	"github.com/agentic-loop/worker/internal/store"
	segmentio "github.com/segmentio/kafka-go"
	tcKafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

type stubCatalog struct{}

func (stubCatalog) SearchSkills(context.Context, string) ([]catalog.Skill, error) {
	return nil, nil
}
func (stubCatalog) ToolsForSkills(context.Context, []string) ([]catalog.Tool, error) {
	return nil, nil
}

type stubModel struct{}

func (stubModel) SelectSkills(context.Context, attempt.SkillSelectionCall) (string, error) {
	return `{"intent":"general","skill_names":[],"reason":"none"}`, nil
}
func (stubModel) CallWithTools(context.Context, attempt.ToolCallingCall) (attempt.ToolCallingResult, error) {
	return attempt.ToolCallingResult{Reply: "unused"}, nil
}

type stubTools struct{}

func (stubTools) Execute(context.Context, attempt.ToolHTTPRequest) attempt.ToolHTTPResponse {
	return attempt.ToolHTTPResponse{}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func startKafka(t *testing.T) (brokers []string, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	container, err := tcKafka.Run(ctx, "confluentinc/confluent-local:7.5.0",
		tcKafka.WithClusterID("agentic-loop-test"),
	)
	if err != nil {
		t.Fatalf("start kafka: %v", err)
	}
	brokers, err = container.Brokers(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("brokers: %v", err)
	}
	if err := kafkaworker.EnsureTopics(brokers, 1); err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("ensure topics: %v", err)
	}
	return brokers, func() { _ = container.Terminate(context.Background()) }
}

func TestKafka_happyPathAndDLQ(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker")
	}
	brokers, cleanup := startKafka(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	a := attempt.New(attempt.Deps{
		Catalog: stubCatalog{},
		Model:   stubModel{},
		Tools:   stubTools{},
		Store:   store.NewMemory(),
		Clock:   fixedClock{now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)},
	})
	bus := kafkaworker.NewKafkaBus(brokers, kafkaworker.GroupID+"-it")
	defer bus.Close()
	consumer := &kafkaworker.Consumer{
		Worker: kafkaworker.NewWorker(a, bus),
		Bus:    bus,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- consumer.Run(ctx) }()

	// Give the consumer group a moment to join.
	time.Sleep(2 * time.Second)

	inputWriter := &segmentio.Writer{
		Addr:     segmentio.TCP(brokers...),
		Topic:    kafkaworker.TopicInput,
		Balancer: &segmentio.Hash{},
	}
	defer inputWriter.Close()

	happyPayload, _ := json.Marshal(contract.InputMessage{
		MessageID:           "msg_kafka_happy",
		ConversationID:      "conv_kafka",
		CustomerID:          "cus_kafka",
		Content:             "Hello there",
		ConversationHistory: []contract.HistoryTurn{},
	})
	if err := inputWriter.WriteMessages(ctx, segmentio.Message{
		Key:   []byte("msg_kafka_happy"),
		Value: happyPayload,
	}); err != nil {
		t.Fatalf("produce happy: %v", err)
	}

	badPayload := []byte(`{"message_id":123,"conversation_id":"x"}`)
	if err := inputWriter.WriteMessages(ctx, segmentio.Message{
		Key:   []byte("msg_kafka_bad"),
		Value: badPayload,
	}); err != nil {
		t.Fatalf("produce dlq: %v", err)
	}

	outReader := segmentio.NewReader(segmentio.ReaderConfig{
		Brokers:     brokers,
		Topic:       kafkaworker.TopicOutput,
		GroupID:     "test-output-reader",
		StartOffset: segmentio.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer outReader.Close()

	dlqReader := segmentio.NewReader(segmentio.ReaderConfig{
		Brokers:     brokers,
		Topic:       kafkaworker.TopicDLQ,
		GroupID:     "test-dlq-reader",
		StartOffset: segmentio.FirstOffset,
		MinBytes:    1,
		MaxBytes:    10e6,
	})
	defer dlqReader.Close()

	outCtx, outCancel := context.WithTimeout(ctx, 45*time.Second)
	defer outCancel()
	outMsg, err := outReader.ReadMessage(outCtx)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(outMsg.Key) != "msg_kafka_happy" {
		t.Errorf("output key = %q, want msg_kafka_happy", outMsg.Key)
	}
	var out contract.OutputMessage
	if err := json.Unmarshal(outMsg.Value, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.MessageID != "msg_kafka_happy" {
		t.Errorf("output message_id = %q", out.MessageID)
	}
	if out.Reply == "" {
		t.Error("output reply must be non-empty")
	}

	dlqCtx, dlqCancel := context.WithTimeout(ctx, 45*time.Second)
	defer dlqCancel()
	dlqMsg, err := dlqReader.ReadMessage(dlqCtx)
	if err != nil {
		t.Fatalf("read dlq: %v", err)
	}
	if string(dlqMsg.Key) != "msg_kafka_bad" {
		t.Errorf("dlq key = %q, want msg_kafka_bad", dlqMsg.Key)
	}
	var dl contract.DeadLetter
	if err := json.Unmarshal(dlqMsg.Value, &dl); err != nil {
		t.Fatalf("unmarshal dlq: %v", err)
	}
	if dl.Reason == "" {
		t.Error("dlq reason must be set")
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
	}
}
