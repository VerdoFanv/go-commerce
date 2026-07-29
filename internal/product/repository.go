package product

import (
	"context"
	"errors"

	"github.com/verdofanv/golang-be/internal/domain"
	"gorm.io/gorm"
)

type Repository interface {
	Create(ctx context.Context, product *ProductModel) error
	FindByID(ctx context.Context, id uint) (*ProductModel, error)
	ListByUser(ctx context.Context, userID uint) ([]ProductModel, error)
	Update(ctx context.Context, product *ProductModel) error
	Delete(ctx context.Context, id uint) error
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, product *ProductModel) error {
	return r.db.WithContext(ctx).Create(product).Error
}

func (r *repository) FindByID(ctx context.Context, id uint) (*ProductModel, error) {
	var product ProductModel
	err := r.db.WithContext(ctx).First(&product, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &product, err
}

func (r *repository) ListByUser(ctx context.Context, userID uint) ([]ProductModel, error) {
	var products []ProductModel
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC").
		Find(&products).Error
	return products, err
}

func (r *repository) Update(ctx context.Context, product *ProductModel) error {
	return r.db.WithContext(ctx).Save(product).Error
}

func (r *repository) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&ProductModel{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
