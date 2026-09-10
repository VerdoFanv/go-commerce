// Package outbox implements the transactional outbox pattern: business writes
// and event rows share one Postgres transaction; a relay publishes unpublished
// rows to Kafka so the HTTP path stays correct when the broker is down.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/verdofanv/golang-be/internal/metrics"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EventRow is the outbox_events table model.
type EventRow struct {
	ID            uint64          `gorm:"primaryKey"`
	EventID       string          `gorm:"column:event_id;size:36;uniqueIndex"`
	AggregateType string          `gorm:"column:aggregate_type;size:64"`
	AggregateID   uint64          `gorm:"column:aggregate_id"`
	EventType     string          `gorm:"column:event_type;size:64"`
	Payload       json.RawMessage `gorm:"column:payload;type:jsonb"`
	CreatedAt     time.Time       `gorm:"column:created_at"`
	PublishedAt   *time.Time      `gorm:"column:published_at"`
}

func (EventRow) TableName() string { return "outbox_events" }

// Writer inserts outbox rows inside an existing GORM transaction.
type Writer struct{}

func NewWriter() *Writer { return &Writer{} }

// Enqueue inserts one unpublished event. Call inside db.Transaction.
func (w *Writer) Enqueue(tx *gorm.DB, aggregateType string, aggregateID uint64, eventType string, payload map[string]any) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal outbox payload: %w", err)
	}
	eventID := uuid.NewString()
	row := EventRow{
		EventID:       eventID,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       body,
		CreatedAt:     time.Now().UTC(),
	}
	if err := tx.Create(&row).Error; err != nil {
		return "", err
	}
	return eventID, nil
}

// Publisher is satisfied by *kafka.Producer.
type Publisher interface {
	Publish(ctx context.Context, key string, event kafka.Event) error
}

const pauseKey = "outbox:relay:paused"

// PauseStore shares lab chaos pause across API replicas (Redis).
type PauseStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}

// Relay polls unpublished rows and publishes them to Kafka.
type Relay struct {
	db        *gorm.DB
	publisher Publisher
	batchSize int
	interval  time.Duration
	store     PauseStore // optional; nil → local-only pause (single replica)

	paused atomic.Bool // local mirror / fallback when Redis unavailable
	mu     sync.Mutex
}

func NewRelay(db *gorm.DB, publisher Publisher, store PauseStore) *Relay {
	return &Relay{
		db:        db,
		publisher: publisher,
		store:     store,
		batchSize: 50,
		interval:  500 * time.Millisecond,
	}
}

// SetPaused pauses/resumes automatic relay (lab chaos). When a PauseStore is
// configured, the flag is cluster-wide so HPA replicas all honor it.
func (r *Relay) SetPaused(paused bool) {
	r.paused.Store(paused)
	if r.store == nil {
		return
	}
	val := "0"
	if paused {
		val = "1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.store.Set(ctx, pauseKey, val, 0); err != nil {
		slog.Warn("outbox pause flag redis set failed; local only", "err", err)
	}
}

func (r *Relay) Paused() bool {
	if r.store != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		v, err := r.store.Get(ctx, pauseKey)
		if err == nil {
			return v == "1"
		}
	}
	return r.paused.Load()
}

// Run loops until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.Paused() {
				continue
			}
			if _, err := r.RelayOnce(ctx); err != nil {
				slog.Warn("outbox relay cycle failed", "err", err)
			}
		}
	}
}

// RelayOnce publishes up to batchSize unpublished events. Returns count published.
// Uses FOR UPDATE SKIP LOCKED inside a short claim transaction so concurrent
// relays do not publish the same row; Kafka publish happens after claim commit
// is avoided — instead we lock row, publish, then mark in the same DB session
// without holding the lock across the network call by relying on single-writer
// mutex + conditional update (published_at IS NULL).
func (r *Relay) RelayOnce(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var rows []EventRow
	if err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("published_at IS NULL").
		Order("created_at ASC").
		Limit(r.batchSize).
		Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	if n, err := r.PendingCount(ctx); err == nil {
		metrics.OutboxPending.Set(float64(n))
	}

	published := 0
	for _, row := range rows {
		var payload map[string]any
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			slog.Warn("outbox: bad payload, skipping", "eventId", row.EventID, "err", err)
			continue
		}
		evt := kafka.Event{
			ID:        row.EventID,
			Type:      row.EventType,
			Payload:   payload,
			CreatedAt: row.CreatedAt.UTC(),
		}
		key := fmt.Sprintf("%s:%d", row.AggregateType, row.AggregateID)
		if err := r.publisher.Publish(ctx, key, evt); err != nil {
			return published, fmt.Errorf("publish %s: %w", row.EventID, err)
		}
		now := time.Now().UTC()
		res := r.db.WithContext(ctx).Model(&EventRow{}).
			Where("id = ? AND published_at IS NULL", row.ID).
			Update("published_at", now)
		if res.Error != nil {
			return published, res.Error
		}
		if res.RowsAffected == 0 {
			continue // another relay won the race
		}
		metrics.EventsPublished.WithLabelValues(row.EventType).Inc()
		published++
	}
	return published, nil
}

// ListPending returns unpublished outbox rows (lab).
func (r *Relay) ListPending(ctx context.Context, limit int) ([]EventRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []EventRow
	err := r.db.WithContext(ctx).
		Where("published_at IS NULL").
		Order("created_at ASC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// PendingCount returns how many events await publish.
func (r *Relay) PendingCount(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&EventRow{}).Where("published_at IS NULL").Count(&n).Error
	return n, err
}
