// Package inventory reacts to order lifecycle events to commit or release stock holds.
// Operations are idempotent via inbox (processed_events) + reservation status checks.
package inventory

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/platform/inbox"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"github.com/verdofanv/golang-be/internal/platform/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Handler struct {
	db    *gorm.DB
	cache Cache // optional; nil skips product cache bust on release
}

// Cache invalidates product:{id} after stock is restored.
type Cache interface {
	Del(ctx context.Context, keys ...string) error
}

func NewHandler(db *gorm.DB, cache Cache) *Handler {
	return &Handler{db: db, cache: cache}
}

// Handle applies inventory side effects for paid / cancelled / payment_failed.
func (h *Handler) Handle(ctx context.Context, msg kafka.Message) error {
	switch msg.Event.Type {
	case domain.EventOrderPaid, domain.EventOrderFulfilled,
		domain.EventOrderCancelled, domain.EventOrderPaymentFailed:
	default:
		return nil
	}

	orderID, ok := asUint(msg.Event.Payload["orderId"])
	if !ok || orderID == 0 {
		return fmt.Errorf("%w: inventory event missing orderId", domain.ErrInvalid)
	}

	var released []uint
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claimed, err := inbox.Claim(tx, "inventory", msg.Event.ID, msg.Event.Type)
		if err != nil {
			return err
		}
		if !claimed {
			return nil
		}
		switch msg.Event.Type {
		case domain.EventOrderPaid, domain.EventOrderFulfilled:
			return commitTX(tx, orderID)
		case domain.EventOrderCancelled, domain.EventOrderPaymentFailed:
			ids, err := releaseTX(tx, orderID)
			if err != nil {
				return err
			}
			released = append(released, ids...)
			return nil
		default:
			return nil
		}
	})
	if err != nil {
		return err
	}
	h.bustProductCache(ctx, released...)
	return nil
}

func (h *Handler) bustProductCache(ctx context.Context, ids ...uint) {
	if h.cache == nil || len(ids) == 0 {
		return
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		keys = append(keys, fmt.Sprintf("product:%d", id))
	}
	if len(keys) == 0 {
		return
	}
	if err := h.cache.Del(ctx, keys...); err != nil {
		slog.Warn("product cache invalidate failed", "err", err, "keys", keys)
	}
}

func commitTX(tx *gorm.DB, orderID uint) error {
	now := time.Now().UTC()
	type resv struct {
		ID        uint
		ProductID uint
		Qty       int
	}
	var rows []resv
	if err := tx.Table("inventory_reservations").
		Select("id, product_id, qty").
		Where("order_id = ? AND status = ?", orderID, domain.ReservationHeld).
		Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		oid := orderID
		if err := ledger.Append(tx, row.ProductID, &oid, 0, ledger.ReasonCommit, nil); err != nil {
			return err
		}
	}
	res := tx.Table("inventory_reservations").
		Where("order_id = ? AND status = ?", orderID, domain.ReservationHeld).
		Updates(map[string]any{"status": domain.ReservationCommitted, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	slog.Debug("inventory commit", "orderId", orderID, "rows", res.RowsAffected)
	return nil
}

func releaseTX(tx *gorm.DB, orderID uint) ([]uint, error) {
	type resv struct {
		ID        uint
		ProductID uint
		Qty       int
	}
	var rows []resv
	if err := tx.Table("inventory_reservations").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id, product_id, qty").
		Where("order_id = ? AND status = ?", orderID, domain.ReservationHeld).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		if err := tx.Exec(`UPDATE products SET stock = stock + ?, updated_at = now() WHERE id = ?`, row.Qty, row.ProductID).Error; err != nil {
			return nil, err
		}
		var bal int
		if err := tx.Table("products").Select("stock").Where("id = ?", row.ProductID).Scan(&bal).Error; err != nil {
			return nil, err
		}
		oid := orderID
		if err := ledger.Append(tx, row.ProductID, &oid, row.Qty, ledger.ReasonRelease, &bal); err != nil {
			return nil, err
		}
		if err := tx.Table("inventory_reservations").Where("id = ?", row.ID).
			Updates(map[string]any{"status": domain.ReservationReleased, "updated_at": now}).Error; err != nil {
			return nil, err
		}
		ids = append(ids, row.ProductID)
	}
	return ids, nil
}

func asUint(v any) (uint, bool) {
	switch n := v.(type) {
	case float64:
		return uint(n), true
	case int:
		return uint(n), true
	case int64:
		return uint(n), true
	case uint:
		return n, true
	case uint64:
		return uint(n), true
	default:
		return 0, false
	}
}
