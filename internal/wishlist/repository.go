package wishlist

import (
	"context"
	"errors"
	"strings"

	"github.com/verdofanv/golang-be/internal/domain"
	"gorm.io/gorm"
)

type Repository interface {
	Create(ctx context.Context, row *WishlistModel) error
	ListByUser(ctx context.Context, userID uint) ([]WishlistModel, error)
	Delete(ctx context.Context, userID, id uint) error
	CountByUser(ctx context.Context, userID uint) (int64, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, row *WishlistModel) error {
	err := r.db.WithContext(ctx).Create(row).Error
	if err != nil {
		// unique (user_id, product_id)
		if isUniqueViolation(err) {
			return domain.ErrConflict
		}
		return err
	}
	return nil
}

func (r *repository) ListByUser(ctx context.Context, userID uint) ([]WishlistModel, error) {
	var rows []WishlistModel
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("id DESC").
		Find(&rows).Error
	return rows, err
}

func (r *repository) Delete(ctx context.Context, userID, id uint) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&WishlistModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *repository) CountByUser(ctx context.Context, userID uint) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&WishlistModel{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique")
}
