// Command worker is the event consumer: it reads product domain events from a
// Kafka consumer group, persists an immutable audit trail to MongoDB, retries
// failures with exponential backoff, and dead-letters poison messages.
// It also exposes /metrics on METRICS_PORT for Prometheus scraping.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/verdofanv/golang-be/internal/worker/audit"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/metrics"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"github.com/verdofanv/golang-be/internal/platform/mongo"
	"go.uber.org/fx"
)

func main() {
	_ = godotenv.Load()

	fx.New(
		fx.Provide(config.Load),

		fx.Provide(mongo.Connect),
		fx.Provide(audit.NewMongoStore),
		fx.Provide(func(s *audit.MongoStore) audit.Store { return s }),

		// Worker consumes the products topic in its own consumer group and
		// publishes failures to the DLQ topic.
		fx.Provide(func(cfg config.Config) *kafka.Consumer {
			return kafka.NewConsumer(cfg, cfg.KafkaTopicProducts, cfg.KafkaGroupWorker)
		}),
		fx.Provide(func(cfg config.Config) *kafka.Producer {
			return kafka.NewProducer(cfg, cfg.KafkaTopicDLQ)
		}),
		fx.Provide(audit.NewProcessor),

		fx.Invoke(registerLifecycle),
	).Run()
}

func registerLifecycle(
	lc fx.Lifecycle,
	cfg config.Config,
	consumer *kafka.Consumer,
	dlq *kafka.Producer,
	mdb *mongo.Client,
	store *audit.MongoStore,
	processor *audit.Processor,
) {
	workerCtx, stopWorker := context.WithCancel(context.Background())
	metricsSrv := &http.Server{
		Addr:              ":" + cfg.MetricsPort,
		Handler:           promhttp.Handler(),
		ReadHeaderTimeout: cfg.RequestTimeout,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := kafka.EnsureTopics(cfg, cfg.KafkaTopicProducts, cfg.KafkaTopicDLQ); err != nil {
				slog.Warn("ensure topics failed (broker may still be starting)", "err", err)
			}
			if err := store.EnsureIndexes(ctx); err != nil {
				return fmt.Errorf("audit indexes: %w", err)
			}

			// Prometheus scrape endpoint.
			go func() {
				slog.Info("worker metrics listening", "addr", metricsSrv.Addr)
				if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					slog.Error("metrics server failed", "err", err)
				}
			}()

			// Consume loop: fetch → process (retry/DLQ inside) → commit offset.
			go func() {
				slog.Info("worker consuming", "topic", cfg.KafkaTopicProducts, "group", cfg.KafkaGroupWorker)
				for {
					msg, err := consumer.Fetch(workerCtx)
					if err != nil {
						if workerCtx.Err() != nil {
							slog.Info("worker consume loop stopped")
							return
						}
						slog.Error("kafka fetch failed", "err", err)
						return
					}

					if err := processor.Handle(workerCtx, msg); err != nil {
						// Not committed → Kafka redelivers on rebalance/restart.
						slog.Error("event processing failed, offset not committed", "err", err)
						metrics.EventsDeadLettered.Inc()
						continue
					}
					if err := consumer.Commit(workerCtx, msg); err != nil {
						slog.Warn("offset commit failed", "err", err)
					}
					metrics.EventsConsumed.WithLabelValues(msg.Event.Type, "worker").Inc()
					slog.Info("event audited",
						"type", msg.Event.Type,
						"eventId", msg.Event.ID,
					)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			slog.Info("shutting down worker")
			stopWorker()

			if err := metricsSrv.Shutdown(ctx); err != nil {
				slog.Warn("metrics server shutdown", "err", err)
			}
			if err := consumer.Close(); err != nil {
				slog.Warn("kafka consumer close", "err", err)
			}
			if err := dlq.Close(); err != nil {
				slog.Warn("kafka dlq producer close", "err", err)
			}
			return mdb.Close(ctx)
		},
	})
}
