// Package kafka wraps segmentio/kafka-go with the two roles this system needs:
// a producer with strongest delivery semantics, and a consumer-group reader
// with manual offset commits (at-least-once processing).
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"github.com/verdofanv/golang-be/internal/config"
)

// Event is the canonical envelope for every domain event on the bus.
type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"createdAt"`
}

// NewEvent fills ID and CreatedAt so producers can never emit malformed envelopes.
func NewEvent(eventType string, payload map[string]any) Event {
	return Event{
		ID:        uuid.NewString(),
		Type:      eventType,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	}
}

// Producer publishes events with RequireAll acks — a write is only "done" once
// every in-sync replica has it. Slower per message, correct for a portfolio
// that claims production semantics.
type Producer struct {
	writer *kafka.Writer
}

func NewProducer(cfg config.Config, topic string) *Producer {
	return &Producer{writer: &kafka.Writer{
		Addr:         kafka.TCP(cfg.KafkaBrokers...),
		Topic:        topic,
		RequiredAcks: kafka.RequireAll,
		BatchTimeout: 10 * time.Millisecond,
		Balancer:     &kafka.LeastBytes{},
		Async:        false,
	}}
}

// Publish marshals the event and writes it with the given partitioning key.
func (p *Producer) Publish(ctx context.Context, key string, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	msg := kafka.Message{
		Key:   []byte(key),
		Value: body,
		Headers: []kafka.Header{
			{Key: "eventType", Value: []byte(event.Type)},
			{Key: "eventId", Value: []byte(event.ID)},
		},
	}
	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("kafka write: %w", err)
	}
	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}

// Consumer is a consumer-group reader. Offsets are committed explicitly via
// Commit after the handler succeeds — messages are never auto-committed, so a
// crash mid-processing redelivers instead of losing the event.
type Consumer struct {
	reader *kafka.Reader
	topic  string
	group  string
}

func NewConsumer(cfg config.Config, topic, group string) *Consumer {
	return &Consumer{
		reader: kafka.NewReader(kafka.ReaderConfig{
			Brokers:        cfg.KafkaBrokers,
			Topic:          topic,
			GroupID:        group,
			MinBytes:       1,
			MaxBytes:       10 << 20, // 10MB
			CommitInterval: 0,        // manual commits only
			StartOffset:    kafka.FirstOffset,
		}),
		topic: topic,
		group: group,
	}
}

// Message pairs a decoded event with its raw message for offset commits.
type Message struct {
	Event   Event
	raw     kafka.Message
	rawBody []byte
}

// RawBody returns the original message bytes (used when dead-lettering).
func (m Message) RawBody() []byte {
	return m.rawBody
}

// Fetch reads the next message without committing its offset.
func (c *Consumer) Fetch(ctx context.Context) (Message, error) {
	raw, err := c.reader.FetchMessage(ctx)
	if err != nil {
		return Message{}, err
	}

	var event Event
	if err := json.Unmarshal(raw.Value, &event); err != nil {
		slog.Warn("kafka: undecodable message", "topic", c.topic, "offset", raw.Offset, "err", err)
		event = Event{} // caller decides: treat zero event as poison pill
	}
	return Message{Event: event, raw: raw, rawBody: raw.Value}, nil
}

// Commit marks the message as processed. Call only after side effects succeed.
func (c *Consumer) Commit(ctx context.Context, msg Message) error {
	return c.reader.CommitMessages(ctx, msg.raw)
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}

// EnsureTopics creates topics idempotently so local/dev compose works without
// a separate provisioning step. In production, topics are managed by IaC and
// this call is a no-op safety net.
func EnsureTopics(cfg config.Config, topics ...string) error {
	var conn *kafka.Conn
	var err error
	for _, broker := range cfg.KafkaBrokers {
		conn, err = kafka.Dial("tcp", broker)
		if err == nil {
			break
		}
	}
	if conn == nil {
		return fmt.Errorf("dial any broker %v: %w", cfg.KafkaBrokers, err)
	}
	defer func() { _ = conn.Close() }()

	specs := make([]kafka.TopicConfig, 0, len(topics))
	for _, t := range topics {
		specs = append(specs, kafka.TopicConfig{
			Topic:             t,
			NumPartitions:     3,
			ReplicationFactor: 1, // single-broker dev; raise via IaC in prod
		})
	}
	if err := conn.CreateTopics(specs...); err != nil {
		return fmt.Errorf("create topics: %w", err)
	}
	return nil
}
