// Package payment simulates a payment provider reacting to order.created events.
// This is the *canonical* charge path for the commerce lab (async provider).
// POST /orders/:id/pay is a manual override for teaching fail/timeout outcomes.
package payment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/platform/inbox"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"github.com/verdofanv/golang-be/internal/platform/ledger"
	"github.com/verdofanv/golang-be/internal/platform/outbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Handler struct {
	db     *gorm.DB
	outbox *outbox.Writer
}

func NewHandler(db *gorm.DB, ow *outbox.Writer) *Handler {
	return &Handler{db: db, outbox: ow}
}

type orderRow struct {
	ID      uint    `gorm:"column:id"`
	UserID  uint    `gorm:"column:user_id"`
	Status  string  `gorm:"column:status"`
	Total   float64 `gorm:"column:total"`
	Version int     `gorm:"column:version"`
}

type paymentRow struct {
	ID             uint      `gorm:"primaryKey"`
	OrderID        uint      `gorm:"column:order_id"`
	IdempotencyKey string    `gorm:"column:idempotency_key"`
	Status         string    `gorm:"column:status"`
	Amount         float64   `gorm:"column:amount"`
	ProviderRef    string    `gorm:"column:provider_ref"`
	Attempt        int       `gorm:"column:attempt"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (paymentRow) TableName() string { return "payments" }

// HandleOrderCreated charges the order (simulator). Idempotent on event ID.
func (h *Handler) HandleOrderCreated(ctx context.Context, msg kafka.Message) error {
	orderID, ok := asUint(msg.Event.Payload["orderId"])
	if !ok || orderID == 0 {
		return fmt.Errorf("%w: order.created missing orderId", domain.ErrInvalid)
	}
	attemptKey := "evt:" + msg.Event.ID

	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claimed, err := inbox.Claim(tx, "payment", msg.Event.ID, msg.Event.Type)
		if err != nil {
			return err
		}
		if !claimed {
			return nil // duplicate delivery — skip side effects
		}

		var order orderRow
		if err := tx.Table("orders").Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", orderID).Take(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				slog.Warn("payment: order missing, ack", "orderId", orderID)
				return nil
			}
			return err
		}

		var existing paymentRow
		if err := tx.Where("order_id = ? AND idempotency_key = ?", orderID, attemptKey).First(&existing).Error; err == nil {
			return nil // already processed this event
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if order.Status == domain.OrderPaid || order.Status == domain.OrderFulfilled || order.Status == domain.OrderCancelled {
			return nil
		}

		outcome := "success"
		if v, ok := msg.Event.Payload["simulateOutcome"].(string); ok && v != "" {
			outcome = v
		}

		now := time.Now().UTC()
		pay := paymentRow{
			OrderID:        order.ID,
			IdempotencyKey: attemptKey,
			Amount:         order.Total,
			Attempt:        1,
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		switch outcome {
		case "fail":
			next, err := domain.Transition(order.Status, domain.OrderPaymentFailed)
			if err != nil {
				return nil // illegal transition — treat as already handled
			}
			pay.Status = domain.PaymentFailed
			pay.ProviderRef = "worker-sim-fail"
			if err := tx.Create(&pay).Error; err != nil {
				return err
			}
			if err := tx.Table("orders").Where("id = ?", order.ID).Updates(map[string]any{
				"status": next, "version": order.Version + 1, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := releaseHeld(tx, order.ID); err != nil {
				return err
			}
			_, err = h.outbox.Enqueue(tx, "order", uint64(order.ID), domain.EventOrderPaymentFailed, map[string]any{
				"orderId": order.ID, "userId": order.UserID, "status": next,
			})
			return err
		default:
			next, err := domain.Transition(order.Status, domain.OrderPaid)
			if err != nil {
				return nil
			}
			pay.Status = domain.PaymentSucceeded
			pay.ProviderRef = "worker-sim-ok"
			if err := tx.Create(&pay).Error; err != nil {
				return err
			}
			if err := tx.Table("orders").Where("id = ?", order.ID).Updates(map[string]any{
				"status": next, "version": order.Version + 1, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := commitHeld(tx, order.ID); err != nil {
				return err
			}
			_, err = h.outbox.Enqueue(tx, "order", uint64(order.ID), domain.EventOrderPaid, map[string]any{
				"orderId": order.ID, "userId": order.UserID, "status": next, "total": order.Total,
			})
			return err
		}
	})
}

func releaseHeld(tx *gorm.DB, orderID uint) error {
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

func commitHeld(tx *gorm.DB, orderID uint) error {
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
	return tx.Table("inventory_reservations").
		Where("order_id = ? AND status = ?", orderID, domain.ReservationHeld).
		Updates(map[string]any{"status": domain.ReservationCommitted, "updated_at": now}).Error
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
