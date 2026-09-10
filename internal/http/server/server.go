// Package server is the composition root for the HTTP API: it wires middleware
// order, routes, probes, and the metrics/pprof surface.
package server

import (
	"net/http"
	"net/http/pprof"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/http/auth"
	"github.com/verdofanv/golang-be/internal/http/health"
	"github.com/verdofanv/golang-be/internal/http/lab"
	"github.com/verdofanv/golang-be/internal/http/middleware"
	"github.com/verdofanv/golang-be/internal/http/notify"
	"github.com/verdofanv/golang-be/internal/http/order"
	"github.com/verdofanv/golang-be/internal/http/product"
	"github.com/verdofanv/golang-be/internal/http/wishlist"
	"github.com/verdofanv/golang-be/internal/metrics"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
)

// NewEngine builds the fully-middlewared Gin engine.
func NewEngine(
	cfg config.Config,
	redisClient *appredis.Client,
	authH *auth.Handler,
	productH *product.Handler,
	wishlistH *wishlist.Handler,
	orderH *order.Handler,
	labH *lab.Handler,
	healthH *health.Handler,
	notifyH *notify.Handler,
) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(middleware.RequestID())
	engine.Use(middleware.Tracing("golang-be-api"))
	engine.Use(metrics.Middleware())
	engine.Use(middleware.SecurityHeaders(cfg.EnableHSTS))
	engine.Use(cors.New(cors.Config{
		AllowOrigins: cfg.CORSOrigins,
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "apikey", "X-API-Key", "Idempotency-Key"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	}))
	engine.Use(middleware.Timeout(cfg.RequestTimeout))
	engine.Use(middleware.RequestLogger())

	// Trust proxy headers when behind Traefik / k3s ingress.
	_ = engine.SetTrustedProxies(nil)

	healthH.RegisterRoutes(engine)
	engine.GET("/metrics", gin.WrapH(promhttp.Handler()))

	if !cfg.IsProduction() {
		dbg := engine.Group("/debug/pprof")
		{
			dbg.GET("/", gin.WrapF(pprof.Index))
			dbg.GET("/cmdline", gin.WrapF(pprof.Cmdline))
			dbg.GET("/profile", gin.WrapF(pprof.Profile))
			dbg.GET("/symbol", gin.WrapF(pprof.Symbol))
			dbg.GET("/trace", gin.WrapF(pprof.Trace))
			dbg.GET("/heap", gin.WrapH(pprof.Handler("heap")))
			dbg.GET("/goroutine", gin.WrapH(pprof.Handler("goroutine")))
		}
		// OpenAPI surface is dev/staging only — avoid exposing schema on public prod ingress.
		engine.Static("/docs", "./docs")
	}

	var limiter middleware.RateLimiter
	if redisClient != nil {
		limiter = middleware.NewRedisLimiter(redisClient.Raw())
	}
	api := engine.Group("/api/v1",
		middleware.APIKey(cfg.APIKey),
		middleware.RateLimit(limiter, cfg.RateLimitMax, cfg.RateLimitWindow),
	)
	authH.RegisterRoutes(api, cfg.JWTSecret)
	productH.RegisterRoutes(api, cfg.JWTSecret)
	wishlistH.RegisterRoutes(api, cfg.JWTSecret)
	orderH.RegisterRoutes(api, cfg.JWTSecret)
	// Chaos / ops lab is disabled in production and admin-gated otherwise.
	labH.RegisterRoutes(api, cfg.JWTSecret, !cfg.IsProduction())

	notifyH.RegisterRoutes(engine)

	return engine
}

// NewHTTPServer wraps the Gin engine in a stdlib server for graceful shutdown.
func NewHTTPServer(cfg config.Config, engine *gin.Engine) *http.Server {
	return &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           engine,
		ReadHeaderTimeout: cfg.RequestTimeout,
	}
}
