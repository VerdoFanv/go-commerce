// Package notify implements the real-time fan-out path: Kafka product events
// are consumed by the API process and broadcast to every connected WebSocket
// client. This is the classic "durable log → push" pattern (Kafka → WS).
package notify

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gofiber/websocket/v2"
	"github.com/verdofanv/golang-be/internal/metrics"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
)

// Hub tracks connected WebSocket clients and broadcasts domain events.
type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]struct{})}
}

func (h *Hub) Add(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = struct{}{}
	metrics.WebSocketConnections.Set(float64(len(h.clients)))
}

func (h *Hub) Remove(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
	metrics.WebSocketConnections.Set(float64(len(h.clients)))
}

func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Broadcast sends the event to every client. Slow/broken clients are dropped —
// the stream is best-effort by design; Kafka remains the durable source.
func (h *Hub) Broadcast(event kafka.Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		slog.Warn("ws broadcast marshal failed", "err", err)
		return
	}

	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for conn := range h.clients {
		clients = append(clients, conn)
	}
	h.mu.RUnlock()

	for _, conn := range clients {
		if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			slog.Debug("ws write failed, dropping client", "err", err)
			h.Remove(conn)
			_ = conn.Close()
		}
	}
}

// Notifier consumes product events from Kafka (dedicated consumer group) and
// fans them out to the Hub. Runs inside the API process.
type Notifier struct {
	consumer *kafka.Consumer
	hub      *Hub
}

func NewNotifier(consumer *kafka.Consumer, hub *Hub) *Notifier {
	return &Notifier{consumer: consumer, hub: hub}
}

// Run blocks until ctx is canceled (shutdown) or the consumer dies.
func (n *Notifier) Run(ctx context.Context) {
	slog.Info("notifier listening for product events")
	for {
		msg, err := n.consumer.Fetch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				slog.Info("notifier shutting down")
				return
			}
			slog.Error("notifier fetch failed", "err", err)
			return
		}

		// Poison pill guard: undecodable events are skipped, never broadcast.
		if msg.Event.ID == "" {
			_ = n.consumer.Commit(ctx, msg)
			continue
		}

		n.hub.Broadcast(msg.Event)
		metrics.EventsConsumed.WithLabelValues(msg.Event.Type, "notifier").Inc()
		if err := n.consumer.Commit(ctx, msg); err != nil {
			slog.Warn("notifier commit failed", "err", err)
		}
	}
}
