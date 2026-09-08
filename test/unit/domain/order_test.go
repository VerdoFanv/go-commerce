package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
)

func TestCanTransition_HappyPaths(t *testing.T) {
	require.True(t, domain.CanTransition(domain.OrderPendingPayment, domain.OrderPaid))
	require.True(t, domain.CanTransition(domain.OrderPendingPayment, domain.OrderPaymentFailed))
	require.True(t, domain.CanTransition(domain.OrderPendingPayment, domain.OrderCancelled))
	require.True(t, domain.CanTransition(domain.OrderPaymentFailed, domain.OrderPaid))
	require.True(t, domain.CanTransition(domain.OrderPaid, domain.OrderFulfilled))
}

func TestCanTransition_Illegal(t *testing.T) {
	require.False(t, domain.CanTransition(domain.OrderPaid, domain.OrderCancelled))
	require.False(t, domain.CanTransition(domain.OrderFulfilled, domain.OrderPaid))
	require.False(t, domain.CanTransition(domain.OrderCancelled, domain.OrderPaid))
}

func TestTransition_ErrorWrapsConflict(t *testing.T) {
	_, err := domain.Transition(domain.OrderPaid, domain.OrderCancelled)
	require.ErrorIs(t, err, domain.ErrConflict)
}
