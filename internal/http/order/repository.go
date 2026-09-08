package order

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/platform/ledger"
	"github.com/verdofanv/golang-be/internal/platform/outbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository persists orders and related commerce rows.
type Repository interface {
	CreateOrder(ctx context.Context, in CreateOrderTX) (*OrderModel, error)
	FindByID(ctx context.Context, userID, id uint) (*OrderModel, []OrderItemModel, error)
	ListByUser(ctx context.Context, userID uint, limit int) ([]OrderModel, error)
	Cancel(ctx context.Context, userID, id uint) (*OrderModel, error)
	Pay(ctx context.Context, userID, id uint, outcome string, attemptKey string) (*OrderModel, *PaymentModel, error)
	Fulfill(ctx context.Context, userID, id uint) (*OrderModel, error)
	FindIdempotency(ctx context.Context, userID uint, key string) (*IdempotencyModel, error)
	SaveIdempotencyResponse(ctx context.Context, userID uint, key, method, path, reqHash string, status int, body []byte) error
}

type CreateOrderTX struct {
	UserID uint
	Items  []CreateItem
}

type CreateItem struct {
	ProductID uint
	Qty       int
}

type repository struct {
	db     *gorm.DB
	outbox *outbox.Writer
}

func NewRepository(db *gorm.DB, ow *outbox.Writer) Repository {
	return &repository{db: db, outbox: ow}
}

func (r *repository) CreateOrder(ctx context.Context, in CreateOrderTX) (*OrderModel, error) {
	var created *OrderModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(in.Items) == 0 {
			return domain.ErrInvalid
		}

		var total float64
		type line struct {
			productID uint
			qty       int
			price     float64
		}
		lines := make([]line, 0, len(in.Items))

		for _, it := range in.Items {
			if it.ProductID == 0 || it.Qty <= 0 {
				return domain.ErrInvalid
			}
			var p ProductStock
			if err := tx.Table("products").
				Select("id, price, stock, name").
				Where("id = ? AND deleted_at IS NULL", it.ProductID).
				Take(&p).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return domain.ErrNotFound
				}
				return err
			}
			res := tx.Exec(
				`UPDATE products SET stock = stock - ?, updated_at = now()
				 WHERE id = ? AND deleted_at IS NULL AND stock >= ?`,
				it.Qty, it.ProductID, it.Qty,
			)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return fmt.Errorf("%w: insufficient stock for product %d", domain.ErrConflict, it.ProductID)
			}
			lines = append(lines, line{productID: it.ProductID, qty: it.Qty, price: p.Price})
			total += p.Price * float64(it.Qty)
		}

		now := time.Now().UTC()
		order := &OrderModel{
			UserID:    in.UserID,
			Status:    domain.OrderPendingPayment,
			Total:     total,
			Currency:  "IDR",
			Version:   1,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.Create(order).Error; err != nil {
			return err
		}

		for _, ln := range lines {
			item := OrderItemModel{
				OrderID:   order.ID,
				ProductID: ln.productID,
				Qty:       ln.qty,
				UnitPrice: ln.price,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			resv := ReservationModel{
				OrderID:   order.ID,
				ProductID: ln.productID,
				Qty:       ln.qty,
				Status:    domain.ReservationHeld,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := tx.Create(&resv).Error; err != nil {
				return err
			}
			var bal int
			if err := tx.Table("products").Select("stock").Where("id = ?", ln.productID).Scan(&bal).Error; err != nil {
				return err
			}
			oid := order.ID
			if err := ledger.Append(tx, ln.productID, &oid, -ln.qty, ledger.ReasonHold, &bal); err != nil {
				return err
			}
		}

		payload := map[string]any{
			"orderId":  order.ID,
			"userId":   order.UserID,
			"total":    order.Total,
			"currency": order.Currency,
			"status":   order.Status,
		}
		if _, err := r.outbox.Enqueue(tx, "order", uint64(order.ID), domain.EventOrderCreated, payload); err != nil {
			return err
		}
		created = order
		return nil
	})
	return created, err
}

func (r *repository) FindByID(ctx context.Context, userID, id uint) (*OrderModel, []OrderItemModel, error) {
	var order OrderModel
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, domain.ErrNotFound
		}
		return nil, nil, err
	}
	var items []OrderItemModel
	if err := r.db.WithContext(ctx).Where("order_id = ?", id).Find(&items).Error; err != nil {
		return nil, nil, err
	}
	return &order, items, nil
}

func (r *repository) ListByUser(ctx context.Context, userID uint, limit int) ([]OrderModel, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var rows []OrderModel
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *repository) Cancel(ctx context.Context, userID, id uint) (*OrderModel, error) {
	var out *OrderModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order OrderModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		next, err := domain.Transition(order.Status, domain.OrderCancelled)
		if err != nil {
			return err
		}
		if err := releaseReservations(tx, order.ID); err != nil {
			return err
		}
		now := time.Now().UTC()
		order.Status = next
		order.Version++
		order.UpdatedAt = now
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		payload := map[string]any{
			"orderId": order.ID,
			"userId":  order.UserID,
			"status":  order.Status,
		}
		if _, err := r.outbox.Enqueue(tx, "order", uint64(order.ID), domain.EventOrderCancelled, payload); err != nil {
			return err
		}
		out = &order
		return nil
	})
	return out, err
}

// Pay applies a simulated payment outcome inside a transaction (manual API path).
func (r *repository) Pay(ctx context.Context, userID, id uint, outcome, attemptKey string) (*OrderModel, *PaymentModel, error) {
	var outOrder *OrderModel
	var outPay *PaymentModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order OrderModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}

		// Idempotent: existing succeeded payment for this key.
		var existing PaymentModel
		if err := tx.Where("order_id = ? AND idempotency_key = ?", id, attemptKey).First(&existing).Error; err == nil {
			outOrder = &order
			outPay = &existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if order.Status == domain.OrderPaid || order.Status == domain.OrderFulfilled {
			outOrder = &order
			return nil
		}

		now := time.Now().UTC()
		pay := PaymentModel{
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
				return err
			}
			pay.Status = domain.PaymentFailed
			pay.ProviderRef = "sim-fail"
			if err := tx.Create(&pay).Error; err != nil {
				return err
			}
			order.Status = next
			order.Version++
			order.UpdatedAt = now
			if err := tx.Save(&order).Error; err != nil {
				return err
			}
			if err := releaseReservations(tx, order.ID); err != nil {
				return err
			}
			payload := map[string]any{"orderId": order.ID, "userId": order.UserID, "status": order.Status}
			if _, err := r.outbox.Enqueue(tx, "order", uint64(order.ID), domain.EventOrderPaymentFailed, payload); err != nil {
				return err
			}
		case "timeout":
			return domain.ErrUnavailable
		default: // success
			next, err := domain.Transition(order.Status, domain.OrderPaid)
			if err != nil {
				return err
			}
			pay.Status = domain.PaymentSucceeded
			pay.ProviderRef = "sim-ok"
			if err := tx.Create(&pay).Error; err != nil {
				return err
			}
			order.Status = next
			order.Version++
			order.UpdatedAt = now
			if err := tx.Save(&order).Error; err != nil {
				return err
			}
			if err := commitReservations(tx, order.ID); err != nil {
				return err
			}
			payload := map[string]any{"orderId": order.ID, "userId": order.UserID, "status": order.Status, "total": order.Total}
			if _, err := r.outbox.Enqueue(tx, "order", uint64(order.ID), domain.EventOrderPaid, payload); err != nil {
				return err
			}
		}
		outOrder = &order
		outPay = &pay
		return nil
	})
	return outOrder, outPay, err
}

func releaseReservations(tx *gorm.DB, orderID uint) error {
	var rows []ReservationModel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ? AND status = ?", orderID, domain.ReservationHeld).
		Find(&rows).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, row := range rows {
		if err := tx.Exec(
			`UPDATE products SET stock = stock + ?, updated_at = now() WHERE id = ?`,
			row.Qty, row.ProductID,
		).Error; err != nil {
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
		if err := tx.Model(&ReservationModel{}).Where("id = ?", row.ID).
			Updates(map[string]any{"status": domain.ReservationReleased, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func commitReservations(tx *gorm.DB, orderID uint) error {
	now := time.Now().UTC()
	var rows []ReservationModel
	if err := tx.Where("order_id = ? AND status = ?", orderID, domain.ReservationHeld).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		oid := orderID
		if err := ledger.Append(tx, row.ProductID, &oid, 0, ledger.ReasonCommit, nil); err != nil {
			return err
		}
	}
	return tx.Model(&ReservationModel{}).
		Where("order_id = ? AND status = ?", orderID, domain.ReservationHeld).
		Updates(map[string]any{"status": domain.ReservationCommitted, "updated_at": now}).Error
}

func (r *repository) Fulfill(ctx context.Context, userID, id uint) (*OrderModel, error) {
	var out *OrderModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var order OrderModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND user_id = ?", id, userID).
			First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		next, err := domain.Transition(order.Status, domain.OrderFulfilled)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		order.Status = next
		order.Version++
		order.UpdatedAt = now
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		payload := map[string]any{"orderId": order.ID, "userId": order.UserID, "status": order.Status}
		if _, err := r.outbox.Enqueue(tx, "order", uint64(order.ID), domain.EventOrderFulfilled, payload); err != nil {
			return err
		}
		out = &order
		return nil
	})
	return out, err
}

func (r *repository) FindIdempotency(ctx context.Context, userID uint, key string) (*IdempotencyModel, error) {
	var row IdempotencyModel
	err := r.db.WithContext(ctx).Where("user_id = ? AND key = ?", userID, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *repository) SaveIdempotencyResponse(ctx context.Context, userID uint, key, method, path, reqHash string, status int, body []byte) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing IdempotencyModel
		err := tx.Where("user_id = ? AND key = ?", userID, key).First(&existing).Error
		if err == nil {
			if existing.ResponseStatus != nil {
				return nil // already stored
			}
			return tx.Model(&existing).Updates(map[string]any{
				"response_status": status,
				"response_body":   body,
			}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		row := IdempotencyModel{
			Key:            key,
			UserID:         userID,
			Method:         method,
			Path:           path,
			RequestHash:    reqHash,
			ResponseStatus: &status,
			ResponseBody:   body,
			CreatedAt:      time.Now().UTC(),
		}
		return tx.Create(&row).Error
	})
}
