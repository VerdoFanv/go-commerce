// Command worker is the event consumer: audit trail, payment simulation, and
// inventory commit/release. Offsets commit only after handlers succeed.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/metrics"
	"github.com/verdofanv/golang-be/internal/platform/database"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"github.com/verdofanv/golang-be/internal/platform/mongo"
	"github.com/verdofanv/golang-be/internal/platform/outbox"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"github.com/verdofanv/golang-be/internal/worker/audit"
	"github.com/verdofanv/golang-be/internal/worker/dispatch"
	"github.com/verdofanv/golang-be/internal/worker/inventory"
	"github.com/verdofanv/golang-be/internal/worker/payment"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

func main() {
	_ = godotenv.Load()

	fx.New(
		fx.Provide(config.Load),

		fx.Provide(database.Connect),
		fx.Provide(appredis.Connect),
		fx.Provide(mongo.Connect),
		fx.Provide(audit.NewMongoStore),
		fx.Provide(func(s *audit.MongoStore) audit.Store { return s }),
		fx.Provide(outbox.NewWriter),
		fx.Provide(payment.NewHandler),
		fx.Provide(func(db *gorm.DB, cache *appredis.Client) *inventory.Handler {
			return inventory.NewHandler(db, cache)
		}),

		fx.Provide(func(cfg config.Config) *kafka.Consumer {
			return kafka.NewConsumer(cfg, cfg.KafkaTopicProducts, cfg.KafkaGroupWorker)
		}),
		fx.Provide(func(cfg config.Config) *kafka.Producer {
			return kafka.NewProducer(cfg, cfg.KafkaTopicDLQ)
		}),
		fx.Provide(audit.NewProcessor),
		fx.Provide(dispatch.NewProcessor),

		fx.Invoke(registerLifecycle),
	).Run()
}

func registerLifecycle(
	lc fx.Lifecycle,
	cfg config.Config,
	db *gorm.DB,
	consumer *kafka.Consumer,
	dlq *kafka.Producer,
	mdb *mongo.Client,
	store *audit.MongoStore,
	processor *dispatch.Processor,
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
				return fmt.Errorf("audit indexes: %w — if Unauthorized, set Secret MONGO_URI with auth (see k8s/secret.example.yaml)", err)
			}

			go func() {
				slog.Info("worker metrics listening", "addr", metricsSrv.Addr)
				if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					slog.Error("metrics server failed", "err", err)
				}
			}()

			go func() {
				slog.Info("worker consuming", "topic", cfg.KafkaTopicProducts, "group", cfg.KafkaGroupWorker)
				backoff := time.Second
				for {
					msg, err := consumer.Fetch(workerCtx)
					if err != nil {
						if workerCtx.Err() != nil {
							slog.Info("worker consume loop stopped")
							return
						}
						slog.Error("kafka fetch failed; will retry", "err", err, "backoff", backoff)
						select {
						case <-workerCtx.Done():
							return
						case <-time.After(backoff):
						}
						if backoff < 30*time.Second {
							backoff *= 2
						}
						continue
					}
					backoff = time.Second

					if err := processor.Handle(workerCtx, msg); err != nil {
						slog.Error("event processing failed, offset not committed", "err", err)
						metrics.EventsDeadLettered.Inc()
						continue
					}
					if err := consumer.Commit(workerCtx, msg); err != nil {
						slog.Warn("offset commit failed", "err", err)
					}
					metrics.EventsConsumed.WithLabelValues(msg.Event.Type, "worker").Inc()
					slog.Info("event processed",
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
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
			return mdb.Close(ctx)
		},
	})
}
