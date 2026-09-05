package mocks

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/product"
)

// ProductRepository is an in-memory product.Repository for unit/integration tests.
type ProductRepository struct {
	mu       sync.RWMutex
	products map[uint]*product.ProductModel
	nextID   uint
}

func NewProductRepository() *ProductRepository {
	return &ProductRepository{
		products: make(map[uint]*product.ProductModel),
		nextID:   1,
	}
}

func (m *ProductRepository) Create(_ context.Context, p *product.ProductModel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p.ID = m.nextID
	m.nextID++
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	cp := *p
	m.products[p.ID] = &cp
	return nil
}

func (m *ProductRepository) FindByID(_ context.Context, id uint) (*product.ProductModel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.products[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

// ListByUserCursor mirrors the SQL implementation: id < cursor, newest first, limited.
func (m *ProductRepository) ListByUserCursor(_ context.Context, userID, cursor uint, limit int) ([]product.ProductModel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]product.ProductModel, 0)
	for _, p := range m.products {
		if p.UserID != userID {
			continue
		}
		if cursor > 0 && p.ID >= cursor {
			continue
		}
		out = append(out, *p)
	}

	// newest first
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })

	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *ProductRepository) Update(_ context.Context, p *product.ProductModel) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.products[p.ID]; !ok {
		return domain.ErrNotFound
	}
	p.UpdatedAt = time.Now().UTC()
	cp := *p
	m.products[p.ID] = &cp
	return nil
}

func (m *ProductRepository) Delete(_ context.Context, id uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.products[id]; !ok {
		return domain.ErrNotFound
	}
	delete(m.products, id)
	return nil
}
