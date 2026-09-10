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
	"github.com/verdofanv/golang-be/internal/metrics"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"github.com/verdofanv/golang-be/internal/platform/typesense"
)

// EventPublisher is satisfied by *kafka.Producer (prod) and test fakes.
type EventPublisher interface {
	Publish(ctx context.Context, key string, event kafka.Event) error
}

// SearchEngine is satisfied by *typesense.Client (prod) and test fakes.
type SearchEngine interface {
	IndexProduct(ctx context.Context, p domain.Product) error
	DeleteProduct(ctx context.Context, id uint) error
	Search(ctx context.Context, query string, limit int) ([]typesense.ProductDocument, error)
}

// Service holds business rules. Every dependency behind an interface is
// nil-tolerant so unit tests run without infrastructure.
type Service struct {
	repo      Repository
	cache     *appredis.Client
	publisher EventPublisher
	search    SearchEngine
	cfg       config.Config
}

func NewService(repo Repository, cache *appredis.Client, publisher EventPublisher, search SearchEngine, cfg config.Config) *Service {
	return &Service{repo: repo, cache: cache, publisher: publisher, search: search, cfg: cfg}
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

// ListResult is the cursor-paginated list payload.
type ListResult struct {
	Items      []domain.Product `json:"items"`
	NextCursor uint             `json:"nextCursor,omitempty"`
	HasMore    bool             `json:"hasMore"`
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

	// Side effects are async: the HTTP response never waits on Kafka/ES.
	s.publishAsync(domain.EventProductCreated, product)
	s.indexAsync(product)

	return &product, nil
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

// List implements keyset (cursor) pagination: stable under concurrent writes,
// O(log n) at any depth — unlike OFFSET which scans every skipped row.
func (s *Service) List(ctx context.Context, userID, cursor uint, limit int) (*ListResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Only the first page is cacheable; cursor pages change too cheaply to cache well.
	if cursor == 0 {
		if cached, err := s.getCachedFirstPage(ctx, userID); err == nil && cached != nil {
			return cached, nil
		}
	}

	// Fetch one extra row as the hasMore sentinel.
	models, err := s.repo.ListByUserCursor(ctx, userID, cursor, limit+1)
	if err != nil {
		return nil, err
	}

	hasMore := len(models) > limit
	if hasMore {
		models = models[:limit]
	}

	items := make([]domain.Product, 0, len(models))
	for i := range models {
		items = append(items, toDomain(&models[i]))
	}

	result := &ListResult{Items: items, HasMore: hasMore}
	if hasMore && len(items) > 0 {
		result.NextCursor = items[len(items)-1].ID
	}

	if cursor == 0 {
		_ = s.cacheFirstPage(ctx, userID, result)
	}
	return result, nil
}

// Catalog lists the public marketplace catalog (all sellers), cursor-paginated.
func (s *Service) Catalog(ctx context.Context, cursor uint, limit int) (*ListResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	models, err := s.repo.ListCatalogCursor(ctx, cursor, limit+1)
	if err != nil {
		return nil, err
	}

	hasMore := len(models) > limit
	if hasMore {
		models = models[:limit]
	}

	items := make([]domain.Product, 0, len(models))
	for i := range models {
		items = append(items, toDomain(&models[i]))
	}

	result := &ListResult{Items: items, HasMore: hasMore}
	if hasMore && len(items) > 0 {
		result.NextCursor = items[len(items)-1].ID
	}
	return result, nil
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
	s.publishAsync(domain.EventProductUpdated, product)
	s.indexAsync(product)
	return &product, nil
}

func (s *Service) Delete(ctx context.Context, userID uint, role string, id uint) error {
	model, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	// RBAC: owners delete their own products; admins delete anything (moderation).
	if model.UserID != userID && role != domain.RoleAdmin {
		return domain.ErrForbidden
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.deleteCache(ctx, id)
	_ = s.invalidateListCache(ctx, userID)

	deleted := toDomain(model)
	s.publishAsync(domain.EventProductDeleted, deleted)
	s.deleteIndexAsync(id)
	return nil
}

// Search delegates full-text queries to typesense. When search is disabled
// (no ES configured) the handler surfaces 503 — an honest degradation.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]typesense.ProductDocument, error) {
	if s.search == nil {
		return nil, domain.ErrUnavailable
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, domain.ErrInvalid
	}
	return s.search.Search(ctx, query, limit)
}

// publishAsync fires an event without blocking the request. The context is
// detached on purpose — the request ctx dies when the handler returns.
func (s *Service) publishAsync(eventType string, p domain.Product) {
	if s.publisher == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		evt := kafka.NewEvent(eventType, map[string]any{
			"id":     p.ID,
			"userId": p.UserID,
			"name":   p.Name,
			"price":  p.Price,
			"stock":  p.Stock,
		})
		key := fmt.Sprintf("%d", p.ID)
		if err := s.publisher.Publish(ctx, key, evt); err != nil {
			slog.Warn("publish event failed", "type", eventType, "err", err, "productId", p.ID)
			return
		}
		metrics.EventsPublished.WithLabelValues(eventType).Inc()
		slog.Info("event published", "type", eventType, "productId", p.ID)
	}()
}

func (s *Service) indexAsync(p domain.Product) {
	if s.search == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.search.IndexProduct(ctx, p); err != nil {
			slog.Warn("search index failed", "err", err, "productId", p.ID)
		}
	}()
}

func (s *Service) deleteIndexAsync(id uint) {
	if s.search == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.search.DeleteProduct(ctx, id); err != nil {
			slog.Warn("search delete failed", "err", err, "productId", id)
		}
	}()
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

// InvalidateProducts drops per-id product cache entries after stock mutates outside
// this service (order hold/release). Without this, GET /products/:id can serve
// stale stock for ProductCacheTTL and hide oversell/teardown truth.
func (s *Service) InvalidateProducts(ctx context.Context, ids ...uint) error {
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if err := s.deleteCache(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// ProductCacheKey is shared with workers that mutate stock outside the product service.
func ProductCacheKey(id uint) string {
	return productCacheKey(id)
}

func (s *Service) invalidateListCache(ctx context.Context, userID uint) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Del(ctx, listCacheKey(userID))
}

func (s *Service) getCachedFirstPage(ctx context.Context, userID uint) (*ListResult, error) {
	if s.cache == nil {
		return nil, redis.Nil
	}
	raw, err := s.cache.Get(ctx, listCacheKey(userID))
	if err != nil {
		return nil, err
	}
	var result ListResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	if result.Items == nil {
		result.Items = []domain.Product{}
	}
	return &result, nil
}

func (s *Service) cacheFirstPage(ctx context.Context, userID uint, result *ListResult) error {
	if s.cache == nil {
		return nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.cache.Set(ctx, listCacheKey(userID), b, s.cfg.ProductCacheTTL)
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
