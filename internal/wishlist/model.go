package wishlist

import (
	"time"
)

// WishlistModel maps the wishlists table (user ↔ product many-to-many with note).
type WishlistModel struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"not null"`
	ProductID uint      `gorm:"not null"`
	Note      string    `gorm:"size:255"`
	CreatedAt time.Time
}

func (WishlistModel) TableName() string { return "wishlists" }

// Item is the API-facing shape (joined product fields filled by service).
type Item struct {
	ID          uint      `json:"id"`
	ProductID   uint      `json:"productId"`
	ProductName string    `json:"productName,omitempty"`
	Price       float64   `json:"price,omitempty"`
	Note        string    `json:"note,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}
