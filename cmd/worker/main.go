package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/platform/rabbitmq"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	mq, err := rabbitmq.Connect(cfg)
	if err != nil {
		slog.Error("rabbitmq", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := mq.Close(); err != nil {
			slog.Warn("rabbitmq close", "err", err)
		}
	}()

	deliveries, err := mq.Consume(cfg.RabbitQueue)
	if err != nil {
		slog.Error("consume", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("worker listening", "queue", cfg.RabbitQueue)

	for {
		select {
		case <-ctx.Done():
			slog.Info("worker shutting down")
			return
		case d, ok := <-deliveries:
			if !ok {
				slog.Warn("delivery channel closed")
				return
			}

			var event rabbitmq.Event
			if err := json.Unmarshal(d.Body, &event); err != nil {
				slog.Error("invalid event payload", "err", err)
				_ = d.Nack(false, false)
				continue
			}

			slog.Info("event received",
				"type", event.Type,
				"payload", event.Payload,
				"createdAt", event.CreatedAt,
			)

			// Contoh side-effect: kirim email, sync search index, audit log, dll.
			if err := d.Ack(false); err != nil {
				slog.Error("ack failed", "err", err)
			}
		}
	}
}
