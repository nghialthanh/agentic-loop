package kafka

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	segmentio "github.com/segmentio/kafka-go"
)

// KafkaBus publishes to output/DLQ topics and commits the pending input offset.
type KafkaBus struct {
	reader       *segmentio.Reader
	outputWriter *segmentio.Writer
	dlqWriter    *segmentio.Writer
	pending      *segmentio.Message
}

// NewKafkaBus builds readers/writers for the locked topic names.
func NewKafkaBus(brokers []string, groupID string) *KafkaBus {
	if groupID == "" {
		groupID = GroupID
	}
	return &KafkaBus{
		reader: segmentio.NewReader(segmentio.ReaderConfig{
			Brokers:        brokers,
			GroupID:        groupID,
			Topic:          TopicInput,
			MinBytes:       1,
			MaxBytes:       10e6,
			CommitInterval: 0, // manual commit after publish
			StartOffset:    segmentio.FirstOffset,
		}),
		outputWriter: &segmentio.Writer{
			Addr:         segmentio.TCP(brokers...),
			Topic:        TopicOutput,
			Balancer:     &segmentio.Hash{},
			RequiredAcks: segmentio.RequireOne,
			Async:        false,
		},
		dlqWriter: &segmentio.Writer{
			Addr:         segmentio.TCP(brokers...),
			Topic:        TopicDLQ,
			Balancer:     &segmentio.Hash{},
			RequiredAcks: segmentio.RequireOne,
			Async:        false,
		},
	}
}

// SetPending stores the fetched input message for a later Commit.
func (b *KafkaBus) SetPending(msg segmentio.Message) {
	m := msg
	b.pending = &m
}

// Publish writes value to topic with key message_id.
func (b *KafkaBus) Publish(ctx context.Context, topic, key string, value []byte) error {
	msg := segmentio.Message{
		Key:   []byte(key),
		Value: value,
		Time:  time.Now().UTC(),
	}
	switch topic {
	case TopicOutput:
		return b.outputWriter.WriteMessages(ctx, msg)
	case TopicDLQ:
		return b.dlqWriter.WriteMessages(ctx, msg)
	default:
		return fmt.Errorf("unknown topic %q", topic)
	}
}

// Commit commits the pending input offset after a successful publish.
func (b *KafkaBus) Commit(ctx context.Context, _ string) error {
	if b.pending == nil {
		return fmt.Errorf("no pending message to commit")
	}
	if err := b.reader.CommitMessages(ctx, *b.pending); err != nil {
		return err
	}
	b.pending = nil
	return nil
}

// Fetch waits for the next input record.
func (b *KafkaBus) Fetch(ctx context.Context) (Record, error) {
	msg, err := b.reader.FetchMessage(ctx)
	if err != nil {
		return Record{}, err
	}
	b.SetPending(msg)
	return Record{
		Key:     string(msg.Key),
		Payload: append([]byte(nil), msg.Value...),
	}, nil
}

// Close shuts down the reader and writers.
func (b *KafkaBus) Close() error {
	var first error
	if err := b.reader.Close(); err != nil && first == nil {
		first = err
	}
	if err := b.outputWriter.Close(); err != nil && first == nil {
		first = err
	}
	if err := b.dlqWriter.Close(); err != nil && first == nil {
		first = err
	}
	return first
}

// EnsureTopics creates the locked topics if they do not exist (dev/test).
func EnsureTopics(brokers []string, partitions int) error {
	if partitions < 1 {
		partitions = 1
	}
	conn, err := segmentio.Dial("tcp", brokers[0])
	if err != nil {
		return err
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return err
	}
	controllerConn, err := segmentio.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		return err
	}
	defer controllerConn.Close()

	topics := []string{TopicInput, TopicOutput, TopicDLQ}
	configs := make([]segmentio.TopicConfig, 0, len(topics))
	for _, t := range topics {
		configs = append(configs, segmentio.TopicConfig{
			Topic:             t,
			NumPartitions:     partitions,
			ReplicationFactor: 1,
		})
	}
	return controllerConn.CreateTopics(configs...)
}

// Consumer runs the fetch → handle loop until ctx is cancelled.
type Consumer struct {
	Worker *Worker
	Bus    *KafkaBus
}

// Run processes messages until the context is done.
// Handle failures leave the offset uncommitted; the same pending record is retried.
func (c *Consumer) Run(ctx context.Context) error {
	var pending *Record
	for {
		if pending == nil {
			rec, err := c.Bus.Fetch(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
			pending = &rec
		}
		if err := c.Worker.Handle(ctx, *pending); err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				// retry without committing
			}
			continue
		}
		pending = nil
	}
}
