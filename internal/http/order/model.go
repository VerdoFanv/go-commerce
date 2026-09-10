package order

import "time"

type OrderModel struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"column:user_id;not null;index"`
	Status    string    `gorm:"size:32;not null"`
	Total     float64   `gorm:"not null"`
	Currency  string    `gorm:"size:8;not null;default:IDR"`
	Version   int       `gorm:"not null;default:1"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (OrderModel) TableName() string { return "orders" }

type OrderItemModel struct {
	ID        uint    `gorm:"primaryKey"`
	OrderID   uint    `gorm:"column:order_id;not null;index"`
	ProductID uint    `gorm:"column:product_id;not null"`
	Qty       int     `gorm:"not null"`
	UnitPrice float64 `gorm:"column:unit_price;not null"`
}

func (OrderItemModel) TableName() string { return "order_items" }

type ReservationModel struct {
	ID        uint      `gorm:"primaryKey"`
	OrderID   uint      `gorm:"column:order_id;not null"`
	ProductID uint      `gorm:"column:product_id;not null"`
	Qty       int       `gorm:"not null"`
	Status    string    `gorm:"size:16;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (ReservationModel) TableName() string { return "inventory_reservations" }

type PaymentModel struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	OrderID        uint      `gorm:"column:order_id;not null;index" json:"orderId"`
	IdempotencyKey string    `gorm:"column:idempotency_key;size:128;not null" json:"idempotencyKey"`
	Status         string    `gorm:"size:32;not null" json:"status"`
	Amount         float64   `gorm:"not null" json:"amount"`
	ProviderRef    string    `gorm:"column:provider_ref;size:128" json:"providerRef"`
	Attempt        int       `gorm:"not null;default:1" json:"attempt"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

func (PaymentModel) TableName() string { return "payments" }

type IdempotencyModel struct {
	ID             uint      `gorm:"primaryKey"`
	Key            string    `gorm:"column:key;size:128;not null"`
	UserID         uint      `gorm:"column:user_id;not null"`
	Method         string    `gorm:"size:16;not null"`
	Path           string    `gorm:"size:255;not null"`
	RequestHash    string    `gorm:"column:request_hash;size:64;not null"`
	ResponseStatus *int      `gorm:"column:response_status"`
	ResponseBody   []byte    `gorm:"column:response_body;type:jsonb"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (IdempotencyModel) TableName() string { return "idempotency_keys" }

type ProductStock struct {
	ID    uint
	Price float64
	Stock int
	Name  string
}
