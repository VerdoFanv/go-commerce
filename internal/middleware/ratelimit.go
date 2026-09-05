package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-redis/redis_rate/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/pkg/response"
)

// RateLimiter abstracts the sliding-window limiter so tests can stub it.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit redis_rate.Limit) (*redis_rate.Result, error)
}

// RateLimit applies a per-IP sliding-window limit backed by Redis (Lua script,
// atomic across replicas). When the limiter is nil (tests, Redis outage at
// boot) the middleware is a pass-through — availability beats strictness here.
func RateLimit(limiter RateLimiter, max int, window time.Duration) fiber.Handler {
	if limiter == nil {
		return func(c *fiber.Ctx) error { return c.Next() }
	}
	limit := redis_rate.Limit{Rate: max, Burst: max, Period: window}

	return func(c *fiber.Ctx) error {
		res, err := limiter.Allow(c.UserContext(), "rl:"+c.IP(), limit)
		if err != nil {
			// Redis hiccup → fail open, log upstream via metrics if needed.
			return c.Next()
		}

		c.Set("X-RateLimit-Limit", itoa(res.Limit.Rate))
		c.Set("X-RateLimit-Remaining", itoa(res.Remaining))
		c.Set("X-RateLimit-Reset", itoa(int(res.ResetAfter.Seconds())))

		if res.Allowed == 0 {
			c.Set("Retry-After", itoa(int(res.RetryAfter.Seconds())+1))
			return response.FailCode(c, http.StatusTooManyRequests,
				domain.ErrRateLimited.Error(), domain.ErrorCode(domain.ErrRateLimited))
		}
		return c.Next()
	}
}

func itoa(v int) string {
	if v < 0 {
		v = 0
	}
	return strconv.Itoa(v)
}

// NewRedisLimiter adapts go-redis to the RateLimiter interface.
func NewRedisLimiter(rdb *redis.Client) RateLimiter {
	return redis_rate.NewLimiter(rdb)
}
