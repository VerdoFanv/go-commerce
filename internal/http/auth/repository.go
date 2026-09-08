package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/verdofanv/golang-be/internal/domain"
	"gorm.io/gorm"
)

type Repository interface {
	Create(ctx context.Context, user *UserModel) error
	FindByEmail(ctx context.Context, email string) (*UserModel, error)
	FindByID(ctx context.Context, id uint) (*UserModel, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) Create(ctx context.Context, user *UserModel) error {
	err := r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrEmailTaken
		}
		return err
	}
	return nil
}

func (r *repository) FindByEmail(ctx context.Context, email string) (*UserModel, error) {
	var user UserModel
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &user, err
}

func (r *repository) FindByID(ctx context.Context, id uint) (*UserModel, error) {
	var user UserModel
	err := r.db.WithContext(ctx).First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	return &user, err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "UNIQUE constraint")
}
