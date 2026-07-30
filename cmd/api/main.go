package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"github.com/verdofanv/golang-be/internal/auth"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/health"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/internal/platform/database"
	"github.com/verdofanv/golang-be/internal/platform/rabbitmq"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"github.com/verdofanv/golang-be/internal/product"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	db, err := database.Connect(cfg)
	if err != nil {
		slog.Error("database", "err", err)
		os.Exit(1)
	}

	if err := db.AutoMigrate(&auth.UserModel{}, &product.ProductModel{}); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}

	redisClient, err := appredis.Connect(cfg)
	if err != nil {
		slog.Error("redis", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			slog.Warn("redis close", "err", err)
		}
	}()

	mqClient, err := rabbitmq.Connect(cfg)
	if err != nil {
		slog.Warn("rabbitmq unavailable, events will be skipped", "err", err)
		mqClient = nil
	} else {
		defer func() {
			if err := mqClient.Close(); err != nil {
				slog.Warn("rabbitmq close", "err", err)
			}
		}()
	}

	authRepo := auth.NewRepository(db)
	authSvc := auth.NewService(authRepo, cfg)
	authH := auth.NewHandler(authSvc)

	productRepo := product.NewRepository(db)
	productSvc := product.NewService(productRepo, redisClient, mqClient, cfg)
	productH := product.NewHandler(productSvc)

	healthH := health.NewHandler(db)

	app := fiber.New(fiber.Config{
		DisableStartupMessage: cfg.AppEnv == "production",
		ReadBufferSize:        4096,
	})
	app.Use(recover.New(), middleware.RequestLogger())

	api := app.Group("/api/v1", middleware.APIKey(cfg.APIKey))
	healthH.RegisterRoutes(api)
	authH.RegisterRoutes(api, cfg.JWTSecret)
	productH.RegisterRoutes(api, cfg.JWTSecret)

	app.Static("/docs", "./docs")
	app.Get("/docs", func(c *fiber.Ctx) error {
		return c.SendFile("./docs/index.html")
	})

	// Goroutine #1: HTTP server jalan parallel, main tetap bisa tunggu signal shutdown.
	go func() {
		addr := ":" + cfg.AppPort
		slog.Info("api listening", "addr", addr, "env", cfg.AppEnv)
		if err := app.Listen(addr); err != nil {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	// Main goroutine block di sini sampai Ctrl+C / SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	slog.Info("shutting down api")
	_ = app.Shutdown()
}
