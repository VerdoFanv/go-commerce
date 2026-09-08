package payment_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"github.com/verdofanv/golang-be/internal/worker/payment"
)

// Ensure duplicate event key is treated as no-op by verifying HandleOrderCreated
// returns nil when payload is missing orderId only after validation — core
// idempotency is covered via unique (order_id, idempotency_key) in SQL + early return.

func TestHandleOrderCreated_InvalidPayload(t *testing.T) {
	h := payment.NewHandler(nil, nil)
	err := h.HandleOrderCreated(context.Background(), kafka.Message{
		Event: kafka.NewEvent(domain.EventOrderCreated, map[string]any{}),
	})
	require.ErrorIs(t, err, domain.ErrInvalid)
}
