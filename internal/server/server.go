// Package server is the composition root for the HTTP API: it wires middleware
// order, routes, probes, and the metrics/pprof surface. Keeping this separate
// from main means tests can build the exact same app as production.
package server

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/verdofanv/golang-be/internal/auth"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/health"
	"github.com/verdofanv/golang-be/internal/lab"
	"github.com/verdofanv/golang-be/internal/metrics"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/internal/notify"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"github.com/verdofanv/golang-be/internal/product"
	"github.com/verdofanv/golang-be/internal/wishlist"
)

// NewApp builds the fully-middlewared Fiber application.
// Middleware order matters: recover → requestid → tracing → metrics → security
// → CORS → compress → timeout → logger → routes.
func NewApp(
	cfg config.Config,
	redisClient *appredis.Client,
	authH *auth.Handler,
	productH *product.Handler,
	wishlistH *wishlist.Handler,
	labH *lab.Handler,
	healthH *health.Handler,
	notifyH *notify.Handler,
) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: cfg.IsProduction(),
		ReadBufferSize:        4096,
		// Trust the platform (LB/Ingress) to terminate; app speaks plain HTTP.
		ProxyHeader: fiber.HeaderXForwardedFor,
	})

	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(middleware.Tracing("golang-be-api"))
	app.Use(metrics.Middleware())
	app.Use(middleware.SecurityHeaders())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // public API; tighten per-environment in real deployments
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, apikey, X-API-Key",
		AllowMethods: "GET, POST, PUT, DELETE, OPTIONS",
	}))
	app.Use(compress.New())
	app.Use(middleware.Timeout(cfg.RequestTimeout))
	app.Use(middleware.RequestLogger())

	// --- Ops surface (no API key: load balancers and scrapers must reach it) ---
	healthH.RegisterRoutes(app)
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))
	if !cfg.IsProduction() {
		app.Use(pprof.New()) // /debug/pprof — dev only
	}

	// --- Business API ---
	var limiter middleware.RateLimiter
	if redisClient != nil {
		limiter = middleware.NewRedisLimiter(redisClient.Raw())
	}
	api := app.Group("/api/v1",
		middleware.APIKey(cfg.APIKey),
		middleware.RateLimit(limiter, cfg.RateLimitMax, cfg.RateLimitWindow),
	)
	authH.RegisterRoutes(api, cfg.JWTSecret)
	productH.RegisterRoutes(api, cfg.JWTSecret)
	wishlistH.RegisterRoutes(api, cfg.JWTSecret)
	labH.RegisterRoutes(api, cfg.JWTSecret)

	// --- Real-time ---
	notifyH.RegisterRoutes(app)

	// --- Docs ---
	app.Static("/docs", "./docs")
	app.Get("/docs", func(c *fiber.Ctx) error {
		return c.SendFile("./docs/index.html")
	})

	return app
}
