package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis_rate/v10"
	"github.com/redis/go-redis/v9"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/pkg/response"
)

// RateLimiter abstracts the sliding-window limiter so tests can stub it.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit redis_rate.Limit) (*redis_rate.Result, error)
}

// RateLimit applies a per-IP sliding-window limit backed by Redis.
func RateLimit(limiter RateLimiter, max int, window time.Duration) gin.HandlerFunc {
	if limiter == nil {
		return func(c *gin.Context) { c.Next() }
	}
	limit := redis_rate.Limit{Rate: max, Burst: max, Period: window}

	return func(c *gin.Context) {
		res, err := limiter.Allow(c.Request.Context(), "rl:"+c.ClientIP(), limit)
		if err != nil {
			// Fail closed on auth routes so Redis outages cannot open credential stuffing.
			if strings.Contains(c.FullPath(), "/authentication/") || strings.Contains(c.Request.URL.Path, "/authentication/") {
				response.FailCode(c, http.StatusServiceUnavailable,
					domain.ErrUnavailable.Error(), domain.ErrorCode(domain.ErrUnavailable))
				c.Abort()
				return
			}
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", itoa(res.Limit.Rate))
		c.Header("X-RateLimit-Remaining", itoa(res.Remaining))
		c.Header("X-RateLimit-Reset", itoa(int(res.ResetAfter.Seconds())))

		if res.Allowed == 0 {
			c.Header("Retry-After", itoa(int(res.RetryAfter.Seconds())+1))
			response.FailCode(c, http.StatusTooManyRequests,
				domain.ErrRateLimited.Error(), domain.ErrorCode(domain.ErrRateLimited))
			c.Abort()
			return
		}
		c.Next()
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
