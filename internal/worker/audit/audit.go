// Package audit implements the worker side of the event-driven architecture:
// every consumed domain event is persisted to MongoDB as an immutable audit
// document. Failed events are retried with backoff, then dead-lettered.
package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/verdofanv/golang-be/internal/platform/kafka"
)

// Record is the immutable audit document written to MongoDB.
type Record struct {
	EventID     string         `bson:"eventId" json:"eventId"`
	Type        string         `bson:"type" json:"type"`
	Payload     map[string]any `bson:"payload" json:"payload"`
	OccurredAt  time.Time      `bson:"occurredAt" json:"occurredAt"`
	ProcessedAt time.Time      `bson:"processedAt" json:"processedAt"`
	Consumer    string         `bson:"consumer" json:"consumer"`
}

// Store abstracts the document sink (MongoDB in prod, fakes in tests).
type Store interface {
	Insert(ctx context.Context, record Record) error
}

const (
	maxRetries     = 3
	retryBaseDelay = 500 * time.Millisecond
)

// Processor consumes events, records them, and dead-letters poison messages.
type Processor struct {
	consumer *kafka.Consumer
	dlq      *kafka.Producer
	store    Store
}

func NewProcessor(consumer *kafka.Consumer, dlq *kafka.Producer, store Store) *Processor {
	return &Processor{consumer: consumer, dlq: dlq, store: store}
}

// Handle processes one fetched message: persist with retries, commit, or DLQ.
// It is split from the read loop so unit tests can drive it without Kafka.
func (p *Processor) Handle(ctx context.Context, msg kafka.Message) error {
	// Poison pill: undecodable envelope → straight to DLQ, no retries wasted.
	if msg.Event.ID == "" {
		return p.deadLetter(ctx, msg, "undecodable event envelope")
	}

	record := Record{
		EventID:     msg.Event.ID,
		Type:        msg.Event.Type,
		Payload:     msg.Event.Payload,
		OccurredAt:  msg.Event.CreatedAt,
		ProcessedAt: time.Now().UTC(),
		Consumer:    "worker",
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := p.store.Insert(ctx, record); err != nil {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryBaseDelay << (attempt - 1)): // exponential backoff
			}
			continue
		}
		return nil // persisted — caller commits the offset
	}
	return p.deadLetter(ctx, msg, fmt.Sprintf("store insert failed after %d attempts: %v", maxRetries, lastErr))
}

// deadLetter publishes the raw message to the DLQ topic with failure context.
func (p *Processor) deadLetter(ctx context.Context, msg kafka.Message, reason string) error {
	if p.dlq == nil {
		return fmt.Errorf("dead letter (%s) but no DLQ producer configured", reason)
	}
	payload := map[string]any{
		"reason":     reason,
		"rawMessage": string(msg.RawBody()),
		"failedAt":   time.Now().UTC(),
	}
	evt := kafka.NewEvent("dlq."+msg.Event.Type, payload)
	if err := p.dlq.Publish(ctx, msg.Event.ID, evt); err != nil {
		return fmt.Errorf("publish to DLQ: %w", err)
	}
	return nil
}
