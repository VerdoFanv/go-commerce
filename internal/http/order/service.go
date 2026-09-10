package order

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/domain"
)

// ProductCache drops Redis product:{id} entries when order flows mutate stock.
type ProductCache interface {
	InvalidateProducts(ctx context.Context, ids ...uint) error
}

type Service struct {
	repo  Repository
	cfg   config.Config
	cache ProductCache // optional; nil in tests
}

func NewService(repo Repository, cfg config.Config, cache ProductCache) *Service {
	return &Service{repo: repo, cfg: cfg, cache: cache}
}

type CreateInput struct {
	UserID         uint
	Items          []CreateItemInput
	IdempotencyKey string
	Method         string
	Path           string
}

type CreateItemInput struct {
	ProductID uint `json:"productId" validate:"required,gt=0"`
	Qty       int  `json:"qty" validate:"required,gt=0"`
}

type PayInput struct {
	UserID  uint
	OrderID uint
	Outcome string // success | fail | timeout
	Key     string // payment attempt idempotency
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Order, int, []byte, error) {
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if in.IdempotencyKey == "" {
		return nil, 0, nil, fmt.Errorf("%w: Idempotency-Key header required", domain.ErrInvalid)
	}
	if len(in.Items) == 0 {
		return nil, 0, nil, domain.ErrInvalid
	}

	reqHash := hashCreateRequest(in.Items)
	if prev, err := s.repo.FindIdempotency(ctx, in.UserID, in.IdempotencyKey); err != nil {
		return nil, 0, nil, err
	} else if prev != nil && prev.ResponseStatus != nil && len(prev.ResponseBody) > 0 {
		if prev.RequestHash != "" && prev.RequestHash != reqHash {
			return nil, 0, nil, fmt.Errorf("%w: Idempotency-Key reused with different body", domain.ErrConflict)
		}
		return nil, *prev.ResponseStatus, prev.ResponseBody, nil
	}

	items := make([]CreateItem, 0, len(in.Items))
	for _, it := range in.Items {
		items = append(items, CreateItem{ProductID: it.ProductID, Qty: it.Qty})
	}
	model, err := s.repo.CreateOrder(ctx, CreateOrderTX{UserID: in.UserID, Items: items})
	if err != nil {
		return nil, 0, nil, err
	}
	s.invalidateProductIDs(ctx, createItemProductIDs(items)...)
	order, err := s.Get(ctx, in.UserID, model.ID)
	if err != nil {
		return nil, 0, nil, err
	}
	return order, 0, nil, nil
}

func (s *Service) RememberIdempotent(ctx context.Context, userID uint, key, method, path string, items []CreateItemInput, status int, body []byte) error {
	return s.repo.SaveIdempotencyResponse(ctx, userID, key, method, path, hashCreateRequest(items), status, body)
}

func (s *Service) Get(ctx context.Context, userID, id uint) (*domain.Order, error) {
	model, items, err := s.repo.FindByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return toDomain(model, items), nil
}

func (s *Service) List(ctx context.Context, userID uint, limit int) ([]domain.Order, error) {
	rows, err := s.repo.ListByUser(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Order, 0, len(rows))
	for i := range rows {
		out = append(out, *toDomain(&rows[i], nil))
	}
	return out, nil
}

func (s *Service) Cancel(ctx context.Context, userID, id uint) (*domain.Order, error) {
	_, items, _ := s.repo.FindByID(ctx, userID, id)
	model, err := s.repo.Cancel(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	s.invalidateProductIDs(ctx, itemProductIDs(items)...)
	return s.Get(ctx, userID, model.ID)
}

func (s *Service) Pay(ctx context.Context, in PayInput) (*domain.Order, *PaymentModel, error) {
	outcome := strings.ToLower(strings.TrimSpace(in.Outcome))
	if outcome == "" {
		outcome = "success"
	}
	switch outcome {
	case "success", "fail", "timeout":
	default:
		return nil, nil, domain.ErrInvalid
	}
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = fmt.Sprintf("pay-%d", in.OrderID)
	}
	order, pay, err := s.repo.Pay(ctx, in.UserID, in.OrderID, outcome, key)
	if err != nil {
		return nil, nil, err
	}
	full, err := s.Get(ctx, in.UserID, order.ID)
	if err != nil {
		return nil, nil, err
	}
	return full, pay, nil
}

// Fulfill is an admin/ops action: mark a paid order as shipped/fulfilled.
func (s *Service) Fulfill(ctx context.Context, id uint) (*domain.Order, error) {
	model, err := s.repo.Fulfill(ctx, id)
	if err != nil {
		return nil, err
	}
	full, items, err := s.repo.FindByIDAny(ctx, model.ID)
	if err != nil {
		return nil, err
	}
	return toDomain(full, items), nil
}

// ExpireStaleHolds cancels unpaid orders older than OrderHoldTTL and releases stock.
func (s *Service) ExpireStaleHolds(ctx context.Context) (int, error) {
	ttl := s.cfg.OrderHoldTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	cutoff := time.Now().UTC().Add(-ttl)
	ids, err := s.repo.ListExpiredPendingIDs(ctx, cutoff, 50)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		_, items, _ := s.repo.FindByIDAny(ctx, id)
		if _, err := s.repo.CancelSystem(ctx, id); err != nil {
			slog.Warn("hold expiry cancel failed", "orderId", id, "err", err)
			continue
		}
		s.invalidateProductIDs(ctx, itemProductIDs(items)...)
		n++
	}
	return n, nil
}

// RunHoldExpiry periodically releases stock held by unpaid orders past TTL.
func (s *Service) RunHoldExpiry(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := s.ExpireStaleHolds(ctx)
			if err != nil {
				slog.Warn("hold expiry sweep failed", "err", err)
				continue
			}
			if n > 0 {
				slog.Info("expired unpaid order holds", "count", n)
			}
		}
	}
}

func hashCreateRequest(items []CreateItemInput) string {
	b, _ := json.Marshal(items)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *Service) invalidateProductIDs(ctx context.Context, ids ...uint) {
	if s.cache == nil || len(ids) == 0 {
		return
	}
	if err := s.cache.InvalidateProducts(ctx, ids...); err != nil {
		slog.Warn("product cache invalidate failed", "err", err, "ids", ids)
	}
}

func createItemProductIDs(items []CreateItem) []uint {
	out := make([]uint, 0, len(items))
	for _, it := range items {
		out = append(out, it.ProductID)
	}
	return out
}

func itemProductIDs(items []OrderItemModel) []uint {
	out := make([]uint, 0, len(items))
	for _, it := range items {
		out = append(out, it.ProductID)
	}
	return out
}

func toDomain(m *OrderModel, items []OrderItemModel) *domain.Order {
	o := &domain.Order{
		ID:        m.ID,
		UserID:    m.UserID,
		Status:    m.Status,
		Total:     m.Total,
		Currency:  m.Currency,
		Version:   m.Version,
		CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: m.UpdatedAt.UTC().Format(time.RFC3339),
	}
	for _, it := range items {
		o.Items = append(o.Items, domain.OrderItem{
			ProductID: it.ProductID,
			Qty:       it.Qty,
			UnitPrice: it.UnitPrice,
		})
	}
	return o
}
