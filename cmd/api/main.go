package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
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

	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

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
	defer redisClient.Close()

	mqClient, err := rabbitmq.Connect(cfg)
	if err != nil {
		slog.Warn("rabbitmq unavailable, events will be skipped", "err", err)
		mqClient = nil
	} else {
		defer mqClient.Close()
	}

	authRepo := auth.NewRepository(db)
	authSvc := auth.NewService(authRepo, cfg)
	authH := auth.NewHandler(authSvc)

	productRepo := product.NewRepository(db)
	productSvc := product.NewService(productRepo, redisClient, mqClient, cfg)
	productH := product.NewHandler(productSvc)

	healthH := health.NewHandler(db)

	r := gin.New()
	r.Use(gin.Recovery(), middleware.RequestLogger())

	api := r.Group("/api/v1")
	api.Use(middleware.APIKey(cfg.APIKey))
	{
		healthH.RegisterRoutes(api)
		authH.RegisterRoutes(api, cfg.JWTSecret)
		productH.RegisterRoutes(api, cfg.JWTSecret)
	}

	r.StaticFile("/docs/openapi.yaml", "./docs/openapi.yaml")
	r.StaticFile("/docs", "./docs/index.html")
	r.StaticFile("/docs/", "./docs/index.html")

	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Goroutine #1: HTTP server jalan paralel, main tetap bisa tunggu signal shutdown.
	go func() {
		slog.Info("api listening", "addr", srv.Addr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	// Main goroutine block di sini sampai Ctrl+C / SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	slog.Info("shutting down api")
	_ = srv.Shutdown(ctx)
}
