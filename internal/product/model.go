package product

import (
	"time"

	"gorm.io/gorm"
)

type ProductModel struct {
	ID          uint           `gorm:"primaryKey"`
	UserID      uint           `gorm:"index;not null"`
	Name        string         `gorm:"size:180;not null"`
	Description string         `gorm:"type:text"`
	Price       float64        `gorm:"not null"`
	Stock       int            `gorm:"not null;default:0"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (ProductModel) TableName() string {
	return "products"
}
