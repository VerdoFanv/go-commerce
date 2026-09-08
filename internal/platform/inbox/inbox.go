// Package inbox implements consumer-side deduplication (the counterpart to
// transactional outbox). Large systems assume at-least-once delivery; the inbox
// makes side effects effectively once per (consumer, event_id).
package inbox

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Row struct {
	EventID     string    `gorm:"column:event_id;primaryKey"`
	Consumer    string    `gorm:"column:consumer;size:64"`
	EventType   string    `gorm:"column:event_type;size:64"`
	ProcessedAt time.Time `gorm:"column:processed_at"`
}

func (Row) TableName() string { return "processed_events" }

// AlreadyProcessed reports whether this consumer already handled eventID.
func AlreadyProcessed(ctx context.Context, db *gorm.DB, consumer, eventID string) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&Row{}).
		Where("event_id = ? AND consumer = ?", eventID, consumer).
		Count(&n).Error
	return n > 0, err
}

// MarkProcessed inserts the inbox row. Duplicate key → already done (idempotent).
func MarkProcessed(ctx context.Context, db *gorm.DB, consumer, eventID, eventType string) error {
	row := Row{
		EventID:     eventID,
		Consumer:    consumer,
		EventType:   eventType,
		ProcessedAt: time.Now().UTC(),
	}
	err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	return err
}

// Claim inserts inside an open transaction. Returns false if already claimed.
func Claim(tx *gorm.DB, consumer, eventID, eventType string) (bool, error) {
	row := Row{
		EventID:     eventID,
		Consumer:    consumer,
		EventType:   eventType,
		ProcessedAt: time.Now().UTC(),
	}
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	return true, nil
}

// ErrSkip is returned by helpers when work should be skipped (already done).
var ErrSkip = errors.New("inbox: already processed")
