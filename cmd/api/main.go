// Command api is the HTTP edge service. Dependency wiring uses uber/fx:
// constructors declare what they need, the framework resolves the graph,
// and every resource registers its own shutdown hook.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
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

		fx.Provide(database.Connect),
		fx.Provide(appredis.Connect),
		fx.Provide(mongo.Connect),
		fx.Provide(typesense.Connect),
		fx.Provide(func(cfg config.Config) (*telemetry.Provider, error) {
			return telemetry.Setup(context.Background(), cfg, "golang-be-api")
		}),

		fx.Provide(func(cfg config.Config) *kafka.Producer {
			return kafka.NewProducer(cfg, cfg.KafkaTopicProducts)
		}),
		fx.Provide(func(cfg config.Config) *kafka.Consumer {
			return kafka.NewConsumer(cfg, cfg.KafkaTopicProducts, cfg.KafkaGroupNotifier)
		}),

		fx.Provide(func(p *kafka.Producer) product.EventPublisher { return p }),
		fx.Provide(func(p *kafka.Producer) lab.EventPublisher { return p }),
		fx.Provide(func(c *typesense.Client) product.SearchEngine {
			if c == nil {
				return nil
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

		fx.Provide(server.NewEngine),
		fx.Provide(server.NewHTTPServer),

		fx.Invoke(registerLifecycle),
	).Run()
}

func registerLifecycle(
	lc fx.Lifecycle,
	cfg config.Config,
	httpServer *http.Server,
	_ *gin.Engine, // ensure engine is constructed
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
			if err := database.Migrate(db); err != nil {
				return fmt.Errorf("migrate: %w", err)
			}

			if err := kafka.EnsureTopics(cfg, cfg.KafkaTopicProducts, cfg.KafkaTopicDLQ); err != nil {
				slog.Warn("ensure topics failed (broker may still be starting)", "err", err)
			}

			go notifier.Run(notifierCtx)

			go func() {
				slog.Info("api listening", "addr", httpServer.Addr, "env", cfg.AppEnv)
				if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					slog.Error("server failed", "err", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			slog.Info("shutting down api")
			stopNotifier()

			if err := httpServer.Shutdown(ctx); err != nil {
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
