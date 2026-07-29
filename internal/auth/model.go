package auth

import (
	"time"

	"gorm.io/gorm"
)

type UserModel struct {
	ID           uint           `gorm:"primaryKey"`
	Name         string         `gorm:"size:120;not null"`
	Email        string         `gorm:"size:180;uniqueIndex;not null"`
	PasswordHash string         `gorm:"size:255;not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

func (UserModel) TableName() string {
	return "users"
}
