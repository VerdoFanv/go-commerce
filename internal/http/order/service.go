package order

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/verdofanv/golang-be/internal/domain"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type CreateInput struct {
	UserID         uint
	Items          []CreateItemInput
	IdempotencyKey string
	Method         string
	Path           string
}

type CreateItemInput struct {
	ProductID uint `json:"productId"`
	Qty       int  `json:"qty"`
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
	model, err := s.repo.Cancel(ctx, userID, id)
	if err != nil {
		return nil, err
	}
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

func (s *Service) Fulfill(ctx context.Context, userID, id uint) (*domain.Order, error) {
	model, err := s.repo.Fulfill(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, model.ID)
}

func hashCreateRequest(items []CreateItemInput) string {
	b, _ := json.Marshal(items)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func toDomain(m *OrderModel, items []OrderItemModel) *domain.Order {
	o := &domain.Order{
		ID:       m.ID,
		UserID:   m.UserID,
		Status:   m.Status,
		Total:    m.Total,
		Currency: m.Currency,
		Version:  m.Version,
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
