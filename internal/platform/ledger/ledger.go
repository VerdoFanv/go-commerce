package ledger

import (
	"time"

	"gorm.io/gorm"
)

// Reasons mirror large-commerce stock movement vocabulary.
const (
	ReasonHold     = "hold"
	ReasonRelease  = "release"
	ReasonCommit   = "commit" // reservation finalized; delta 0, audit only
	ReasonAdjust   = "adjust"
)

type Entry struct {
	ID           uint64    `gorm:"primaryKey"`
	ProductID    uint      `gorm:"column:product_id"`
	OrderID      *uint     `gorm:"column:order_id"`
	Delta        int       `gorm:"column:delta"`
	Reason       string    `gorm:"column:reason;size:32"`
	BalanceAfter *int      `gorm:"column:balance_after"`
	CreatedAt    time.Time `gorm:"column:created_at"`
}

func (Entry) TableName() string { return "stock_ledger" }

// Append writes one immutable stock movement inside the caller's transaction.
func Append(tx *gorm.DB, productID uint, orderID *uint, delta int, reason string, balanceAfter *int) error {
	e := Entry{
		ProductID:    productID,
		OrderID:      orderID,
		Delta:        delta,
		Reason:       reason,
		BalanceAfter: balanceAfter,
		CreatedAt:    time.Now().UTC(),
	}
	return tx.Create(&e).Error
}
