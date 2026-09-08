// Command api is the HTTP edge service. Dependency wiring uses uber/fx:
// constructors declare what they need, the framework resolves the graph,
// and every resource registers its own shutdown hook.
package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"github.com/verdofanv/golang-be/internal/audit"
	"github.com/verdofanv/golang-be/internal/auth"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/health"
	"github.com/verdofanv/golang-be/internal/lab"
	"github.com/verdofanv/golang-be/internal/notify"
	"github.com/verdofanv/golang-be/internal/platform/database"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"github.com/verdofanv/golang-be/internal/platform/mongo"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"github.com/verdofanv/golang-be/internal/platform/telemetry"
	"github.com/verdofanv/golang-be/internal/platform/typesense"
	"github.com/verdofanv/golang-be/internal/product"
	"github.com/verdofanv/golang-be/internal/server"
	"github.com/verdofanv/golang-be/internal/wishlist"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

func main() {
	_ = godotenv.Load()

	fx.New(
		fx.Provide(config.Load),

		// --- Platform adapters ---
		fx.Provide(database.Connect),
		fx.Provide(appredis.Connect),
		fx.Provide(mongo.Connect),
		fx.Provide(typesense.Connect),
		fx.Provide(func(cfg config.Config) (*telemetry.Provider, error) {
			return telemetry.Setup(context.Background(), cfg, "golang-be-api")
		}),

		// --- Messaging: products producer + notifier consumer group ---
		fx.Provide(func(cfg config.Config) *kafka.Producer {
			return kafka.NewProducer(cfg, cfg.KafkaTopicProducts)
		}),
		fx.Provide(func(cfg config.Config) *kafka.Consumer {
			return kafka.NewConsumer(cfg, cfg.KafkaTopicProducts, cfg.KafkaGroupNotifier)
		}),

		// --- Interface adapters (decouple services from concrete platforms) ---
		fx.Provide(func(p *kafka.Producer) product.EventPublisher { return p }),
		fx.Provide(func(p *kafka.Producer) lab.EventPublisher { return p }),
		fx.Provide(func(c *typesense.Client) product.SearchEngine {
			if c == nil {
				return nil // typed-nil trap: return an untyped nil interface
			}
			return c
		}),
		fx.Provide(func(c *typesense.Client) lab.SearchEngine {
			if c == nil {
				return nil
			}
			return c
		}),
		fx.Provide(func(c *mongo.Client) *audit.MongoStore { return audit.NewMongoStore(c) }),
		fx.Provide(func(s *audit.MongoStore) lab.AuditReader { return s }),

		// --- Domain ---
		fx.Provide(auth.NewRepository),
		fx.Provide(auth.NewService),
		fx.Provide(auth.NewHandler),
		fx.Provide(product.NewRepository),
		fx.Provide(product.NewService),
		fx.Provide(product.NewHandler),
		fx.Provide(wishlist.NewRepository),
		fx.Provide(func(repo wishlist.Repository, products product.Repository, cache *appredis.Client, cfg config.Config) *wishlist.Service {
			return wishlist.NewService(repo, products, cache, cfg)
		}),
		fx.Provide(wishlist.NewHandler),
		fx.Provide(lab.NewService),
		fx.Provide(lab.NewHandler),

		// --- Probes & real-time ---
		fx.Provide(func(db *gorm.DB, rdb *appredis.Client, mdb *mongo.Client, ts *typesense.Client) *health.Handler {
			return health.NewHandler(
				health.PostgresCheck{DB: db},
				health.RedisCheck{Client: rdb},
				health.MongoCheck{Client: mdb},
				health.TypesenseCheck{Client: ts},
			)
		}),
		fx.Provide(notify.NewHub),
		fx.Provide(notify.NewNotifier),
		fx.Provide(notify.NewHandler),

		// --- Composition root ---
		fx.Provide(server.NewApp),

		fx.Invoke(registerLifecycle),
	).Run() // blocks until SIGINT/SIGTERM, then runs OnStop hooks
}

// registerLifecycle owns boot order (migrate → topics → serve) and graceful
// shutdown (stop listeners → drain → close connections).
func registerLifecycle(
	lc fx.Lifecycle,
	cfg config.Config,
	app *fiber.App,
	db *gorm.DB,
	rdb *appredis.Client,
	mdb *mongo.Client,
	producer *kafka.Producer,
	consumer *kafka.Consumer,
	notifier *notify.Notifier,
	tel *telemetry.Provider,
) {
	notifierCtx, stopNotifier := context.WithCancel(context.Background())

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// 1. Schema first — never serve traffic against an unmigrated DB.
			if err := database.Migrate(db); err != nil {
				return fmt.Errorf("migrate: %w", err)
			}

			// 2. Topics are a dev convenience; prod topics come from IaC.
			if err := kafka.EnsureTopics(cfg, cfg.KafkaTopicProducts, cfg.KafkaTopicDLQ); err != nil {
				slog.Warn("ensure topics failed (broker may still be starting)", "err", err)
			}

			// 3. Real-time fan-out: Kafka → WebSocket hub.
			go notifier.Run(notifierCtx)

			// 4. HTTP server last, after everything it serves is ready.
			go func() {
				slog.Info("api listening", "addr", ":"+cfg.AppPort, "env", cfg.AppEnv)
				if err := app.Listen(":" + cfg.AppPort); err != nil {
					slog.Error("server failed", "err", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			slog.Info("shutting down api")
			stopNotifier()

			if err := app.ShutdownWithContext(ctx); err != nil {
				slog.Warn("http shutdown", "err", err)
			}
			if err := consumer.Close(); err != nil {
				slog.Warn("kafka consumer close", "err", err)
			}
			if err := producer.Close(); err != nil {
				slog.Warn("kafka producer close", "err", err)
			}
			if err := mdb.Close(ctx); err != nil {
				slog.Warn("mongo close", "err", err)
			}
			if err := rdb.Close(); err != nil {
				slog.Warn("redis close", "err", err)
			}
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
			return tel.Shutdown(ctx)
		},
	})
}
