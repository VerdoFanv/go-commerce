package domain

import "time"

type Product struct {
	ID          uint      `json:"id"`
	UserID      uint      `json:"userId"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       float64   `json:"price"`
	Stock       int       `json:"stock"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const (
	EventProductCreated = "product.created"
)
