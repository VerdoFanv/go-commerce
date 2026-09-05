package testutil

import (
	"time"

	"github.com/verdofanv/golang-be/internal/config"
	"golang.org/x/crypto/bcrypt"
)

// Config returns a fast, deterministic config for tests.
func Config() config.Config {
	return config.Config{
		AppEnv:          "test",
		AppPort:         "8080",
		APIKey:          "test-api-key",
		JWTSecret:       "test-jwt-secret",
		JWTAccessTTL:    15 * time.Minute,
		JWTRefreshTTL:   24 * time.Hour,
		BcryptCost:      bcrypt.MinCost,
		ProductCacheTTL: 5 * time.Minute,
		RateLimitMax:    100,
		RateLimitWindow: time.Minute,
		RequestTimeout:  10 * time.Second,
		ShutdownTimeout: 5 * time.Second,
		MetricsPort:     "2112",
	}
}
