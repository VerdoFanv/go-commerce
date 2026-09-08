package wishlist

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/domain"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"github.com/verdofanv/golang-be/internal/http/product"
)

// ProductFinder is satisfied by product.Repository (avoids importing concrete cycles via iface).
type ProductFinder interface {
	FindByID(ctx context.Context, id uint) (*product.ProductModel, error)
}

type Service struct {
	repo    Repository
	products ProductFinder
	cache   *appredis.Client
	cfg     config.Config
}

func NewService(repo Repository, products ProductFinder, cache *appredis.Client, cfg config.Config) *Service {
	return &Service{repo: repo, products: products, cache: cache, cfg: cfg}
}

type AddInput struct {
	UserID    uint
	ProductID uint
	Note      string
}

func (s *Service) Add(ctx context.Context, in AddInput) (*Item, error) {
	if in.ProductID == 0 {
		return nil, domain.ErrInvalid
	}
	prod, err := s.products.FindByID(ctx, in.ProductID)
	if err != nil {
		return nil, err
	}

	row := &WishlistModel{
		UserID:    in.UserID,
		ProductID: in.ProductID,
		Note:      strings.TrimSpace(in.Note),
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return nil, err
	}
	_ = s.invalidateCountCache(ctx, in.UserID)

	item := toItem(row, prod.Name, prod.Price)
	return &item, nil
}

func (s *Service) List(ctx context.Context, userID uint) ([]Item, error) {
	rows, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(rows))
	for _, row := range rows {
		name, price := "", 0.0
		if p, err := s.products.FindByID(ctx, row.ProductID); err == nil && p != nil {
			name, price = p.Name, p.Price
		}
		out = append(out, toItem(&row, name, price))
	}
	return out, nil
}

func (s *Service) Remove(ctx context.Context, userID, id uint) error {
	if err := s.repo.Delete(ctx, userID, id); err != nil {
		return err
	}
	_ = s.invalidateCountCache(ctx, userID)
	return nil
}

// Count demonstrates Redis cache-aside for a cheap aggregate.
func (s *Service) Count(ctx context.Context, userID uint) (int64, error) {
	key := countKey(userID)
	if s.cache != nil {
		if raw, err := s.cache.Raw().Get(ctx, key).Result(); err == nil {
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
				return n, nil
			}
		}
	}

	n, err := s.repo.CountByUser(ctx, userID)
	if err != nil {
		return 0, err
	}
	if s.cache != nil {
		_ = s.cache.Raw().Set(ctx, key, fmt.Sprintf("%d", n), s.cfg.ProductCacheTTL).Err()
	}
	return n, nil
}

func (s *Service) invalidateCountCache(ctx context.Context, userID uint) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.Raw().Del(ctx, countKey(userID)).Err()
}

func countKey(userID uint) string {
	return fmt.Sprintf("wishlist:count:%d", userID)
}

func toItem(row *WishlistModel, name string, price float64) Item {
	return Item{
		ID:          row.ID,
		ProductID:   row.ProductID,
		ProductName: name,
		Price:       price,
		Note:        row.Note,
		CreatedAt:   row.CreatedAt.UTC(),
	}
}