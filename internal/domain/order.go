package domain

import "fmt"

// Order lifecycle statuses (state machine).
const (
	OrderPendingPayment = "pending_payment"
	OrderPaid           = "paid"
	OrderPaymentFailed  = "payment_failed"
	OrderCancelled      = "cancelled"
	OrderFulfilled      = "fulfilled"
)

// Inventory reservation statuses.
const (
	ReservationHeld      = "held"
	ReservationCommitted = "committed"
	ReservationReleased  = "released"
)

// Payment statuses.
const (
	PaymentPending = "pending"
	PaymentSucceeded = "succeeded"
	PaymentFailed  = "failed"
)

// Domain event types for the commerce choreography (outbox → Kafka).
const (
	EventOrderCreated       = "order.created"
	EventOrderPaid          = "order.paid"
	EventOrderPaymentFailed = "order.payment_failed"
	EventOrderCancelled     = "order.cancelled"
	EventOrderFulfilled     = "order.fulfilled"
)

// Order is the commerce aggregate returned to API clients.
type Order struct {
	ID        uint        `json:"id"`
	UserID    uint        `json:"userId"`
	Status    string      `json:"status"`
	Total     float64     `json:"total"`
	Currency  string      `json:"currency"`
	Version   int         `json:"version"`
	Items     []OrderItem `json:"items,omitempty"`
	CreatedAt string      `json:"createdAt,omitempty"`
	UpdatedAt string      `json:"updatedAt,omitempty"`
}

type OrderItem struct {
	ProductID uint    `json:"productId"`
	Qty       int     `json:"qty"`
	UnitPrice float64 `json:"unitPrice"`
}

// allowedTransitions encodes the order state machine.
var allowedTransitions = map[string]map[string]struct{}{
	OrderPendingPayment: {
		OrderPaid:          {},
		OrderPaymentFailed: {},
		OrderCancelled:     {},
	},
	OrderPaymentFailed: {
		OrderPaid:      {}, // retry pay
		OrderCancelled: {},
	},
	OrderPaid: {
		OrderFulfilled: {},
	},
}

// CanTransition reports whether from → to is a legal order status change.
func CanTransition(from, to string) bool {
	next, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = next[to]
	return ok
}

// Transition validates and returns the new status, or ErrConflict.
func Transition(from, to string) (string, error) {
	if !CanTransition(from, to) {
		return "", fmt.Errorf("%w: cannot transition order %s → %s", ErrConflict, from, to)
	}
	return to, nil
}
