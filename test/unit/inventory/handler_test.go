package inventory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"github.com/verdofanv/golang-be/internal/worker/inventory"
)

func TestHandle_MissingOrderID(t *testing.T) {
	h := inventory.NewHandler(nil, nil)
	err := h.Handle(context.Background(), kafka.Message{
		Event: kafka.NewEvent(domain.EventOrderPaid, map[string]any{}),
	})
	require.ErrorIs(t, err, domain.ErrInvalid)
}

func TestHandle_UnknownTypeNoop(t *testing.T) {
	h := inventory.NewHandler(nil, nil)
	err := h.Handle(context.Background(), kafka.Message{
		Event: kafka.NewEvent("product.created", map[string]any{"orderId": float64(1)}),
	})
	require.NoError(t, err)
}
