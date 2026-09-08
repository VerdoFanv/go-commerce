// Package dispatch routes Kafka domain events to audit + commerce handlers.
package dispatch

import (
	"context"
	"log/slog"

	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"github.com/verdofanv/golang-be/internal/worker/audit"
	"github.com/verdofanv/golang-be/internal/worker/inventory"
	"github.com/verdofanv/golang-be/internal/worker/payment"
)

// Processor runs commerce side effects then durable audit (or DLQ).
type Processor struct {
	audit     *audit.Processor
	payment   *payment.Handler
	inventory *inventory.Handler
}

func NewProcessor(auditProc *audit.Processor, pay *payment.Handler, inv *inventory.Handler) *Processor {
	return &Processor{audit: auditProc, payment: pay, inventory: inv}
}

func (p *Processor) Handle(ctx context.Context, msg kafka.Message) error {
	if msg.Event.ID != "" {
		if err := p.routeCommerce(ctx, msg); err != nil {
			slog.Error("commerce handler failed", "type", msg.Event.Type, "err", err)
			return err
		}
	}
	return p.audit.Handle(ctx, msg)
}

func (p *Processor) routeCommerce(ctx context.Context, msg kafka.Message) error {
	switch msg.Event.Type {
	case domain.EventOrderCreated:
		if p.payment != nil {
			return p.payment.HandleOrderCreated(ctx, msg)
		}
	case domain.EventOrderPaid, domain.EventOrderCancelled, domain.EventOrderPaymentFailed, domain.EventOrderFulfilled:
		if p.inventory != nil {
			return p.inventory.Handle(ctx, msg)
		}
	}
	return nil
}
