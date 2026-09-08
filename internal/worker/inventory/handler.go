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
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
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

	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
			return releaseTX(tx, orderID)
		default:
			return nil
		}
	})
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

func releaseTX(tx *gorm.DB, orderID uint) error {
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
		return err
	}
	now := time.Now().UTC()
	for _, row := range rows {
		if err := tx.Exec(`UPDATE products SET stock = stock + ?, updated_at = now() WHERE id = ?`, row.Qty, row.ProductID).Error; err != nil {
			return err
		}
		var bal int
		if err := tx.Table("products").Select("stock").Where("id = ?", row.ProductID).Scan(&bal).Error; err != nil {
			return err
		}
		oid := orderID
		if err := ledger.Append(tx, row.ProductID, &oid, row.Qty, ledger.ReasonRelease, &bal); err != nil {
			return err
		}
		if err := tx.Table("inventory_reservations").Where("id = ?", row.ID).
			Updates(map[string]any{"status": domain.ReservationReleased, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	return nil
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
