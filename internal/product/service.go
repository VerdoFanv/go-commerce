package product

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/platform/rabbitmq"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
)

type Service struct {
	repo   Repository
	cache  *appredis.Client
	mq     *rabbitmq.Client
	cfg    config.Config
}

func NewService(repo Repository, cache *appredis.Client, mq *rabbitmq.Client, cfg config.Config) *Service {
	return &Service{repo: repo, cache: cache, mq: mq, cfg: cfg}
}

type CreateInput struct {
	UserID      uint
	Name        string
	Description string
	Price       float64
	Stock       int
}

type UpdateInput struct {
	Name        *string
	Description *string
	Price       *float64
	Stock       *int
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Product, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || in.Price < 0 || in.Stock < 0 {
		return nil, domain.ErrInvalid
	}

	model := &ProductModel{
		UserID:      in.UserID,
		Name:        in.Name,
		Description: strings.TrimSpace(in.Description),
		Price:       in.Price,
		Stock:       in.Stock,
	}
	if err := s.repo.Create(ctx, model); err != nil {
		return nil, err
	}

	product := toDomain(model)
	_ = s.setCache(ctx, product)
	_ = s.invalidateListCache(ctx, in.UserID)

	// Publish event di goroutine supaya response HTTP tidak nunggu RabbitMQ.
	s.publishProductCreatedAsync(product)

	return &product, nil
}

func (s *Service) publishProductCreatedAsync(product domain.Product) {
	if s.mq == nil {
		return
	}

	// Jangan pakai request context — bisa cancel saat handler selesai.
	go func(p domain.Product) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := s.mq.Publish(ctx, domain.EventProductCreated, rabbitmq.Event{
			Type: domain.EventProductCreated,
			Payload: map[string]any{
				"id":     p.ID,
				"userId": p.UserID,
				"name":   p.Name,
				"price":  p.Price,
				"stock":  p.Stock,
			},
		})
		if err != nil {
			slog.Warn("publish product.created failed", "err", err, "productId", p.ID)
			return
		}
		slog.Info("product.created published async", "productId", p.ID)
	}(product)
}

func (s *Service) GetByID(ctx context.Context, id uint) (*domain.Product, error) {
	if cached, err := s.getCache(ctx, id); err == nil && cached != nil {
		return cached, nil
	}

	model, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	product := toDomain(model)
	_ = s.setCache(ctx, product)
	return &product, nil
}

func (s *Service) List(ctx context.Context, userID uint) ([]domain.Product, error) {
	key := listCacheKey(userID)
	if s.cache != nil {
		raw, err := s.cache.Get(ctx, key)
		if err == nil {
			var products []domain.Product
			if json.Unmarshal([]byte(raw), &products) == nil {
				return products, nil
			}
		} else if err != redis.Nil {
			slog.Warn("redis get list failed", "err", err)
		}
	}

	models, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	products := make([]domain.Product, 0, len(models))
	for i := range models {
		products = append(products, toDomain(&models[i]))
	}

	if s.cache != nil {
		if b, err := json.Marshal(products); err == nil {
			_ = s.cache.Set(ctx, key, b, s.cfg.ProductCacheTTL)
		}
	}

	return products, nil
}

func (s *Service) Update(ctx context.Context, userID, id uint, in UpdateInput) (*domain.Product, error) {
	model, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if model.UserID != userID {
		return nil, domain.ErrForbidden
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, domain.ErrInvalid
		}
		model.Name = name
	}
	if in.Description != nil {
		model.Description = strings.TrimSpace(*in.Description)
	}
	if in.Price != nil {
		if *in.Price < 0 {
			return nil, domain.ErrInvalid
		}
		model.Price = *in.Price
	}
	if in.Stock != nil {
		if *in.Stock < 0 {
			return nil, domain.ErrInvalid
		}
		model.Stock = *in.Stock
	}

	if err := s.repo.Update(ctx, model); err != nil {
		return nil, err
	}

	product := toDomain(model)
	_ = s.setCache(ctx, product)
	_ = s.invalidateListCache(ctx, userID)
	return &product, nil
}

func (s *Service) Delete(ctx context.Context, userID, id uint) error {
	model, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if model.UserID != userID {
		return domain.ErrForbidden
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.deleteCache(ctx, id)
	_ = s.invalidateListCache(ctx, userID)
	return nil
}

func (s *Service) getCache(ctx context.Context, id uint) (*domain.Product, error) {
	if s.cache == nil {
		return nil, redis.Nil
	}
	raw, err := s.cache.Get(ctx, productCacheKey(id))
	if err != nil {
		return nil, err
	}
	var product domain.Product
	if err := json.Unmarshal([]byte(raw), &product); err != nil {
		return nil, err
	}
	return &product, nil
}

func (s *Service) setCache(ctx context.Context, product domain.Product) error {
	if s.cache == nil {
		return nil
	}
	b, err := json.Marshal(product)
	if err != nil {
		return err
	}
	return s.cache.Set(ctx, productCacheKey(product.ID), b, s.cfg.ProductCacheTTL)
}

func (s *Service) deleteCache(ctx context.Context, id uint) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Del(ctx, productCacheKey(id))
}

func (s *Service) invalidateListCache(ctx context.Context, userID uint) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Del(ctx, listCacheKey(userID))
}

func productCacheKey(id uint) string {
	return fmt.Sprintf("product:%d", id)
}

func listCacheKey(userID uint) string {
	return fmt.Sprintf("products:user:%d", userID)
}

func toDomain(m *ProductModel) domain.Product {
	return domain.Product{
		ID:          m.ID,
		UserID:      m.UserID,
		Name:        m.Name,
		Description: m.Description,
		Price:       m.Price,
		Stock:       m.Stock,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}
