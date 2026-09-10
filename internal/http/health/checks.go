package health

import (
	"context"

	"github.com/verdofanv/golang-be/internal/platform/mongo"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"github.com/verdofanv/golang-be/internal/platform/typesense"
	"gorm.io/gorm"
)

// PostgresCheck pings via the underlying sql.DB. Critical for readiness.
type PostgresCheck struct{ DB *gorm.DB }

func (p PostgresCheck) Name() string     { return "postgres" }
func (p PostgresCheck) Critical() bool   { return true }
func (p PostgresCheck) Ping(ctx context.Context) error {
	sqlDB, err := p.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// RedisCheck pings the cache / rate-limit / refresh-token store. Critical —
// auth and RL cannot operate safely without it.
type RedisCheck struct{ Client *appredis.Client }

func (r RedisCheck) Name() string   { return "redis" }
func (r RedisCheck) Critical() bool { return true }
func (r RedisCheck) Ping(ctx context.Context) error {
	return r.Client.Raw().Ping(ctx).Err()
}

// MongoCheck pings the audit store. Non-critical for API readiness: commerce
// HTTP works without Mongo; worker owns the audit path. Still reported so
// operators see degradation.
type MongoCheck struct{ Client *mongo.Client }

func (m MongoCheck) Name() string   { return "mongo" }
func (m MongoCheck) Critical() bool { return false }
func (m MongoCheck) Ping(ctx context.Context) error {
	return m.Client.Ping(ctx)
}

// TypesenseCheck pings the search cluster. Non-critical: search returns 503
// via circuit breaker while CRUD/orders stay up. A nil client (search disabled)
// reports healthy so probes don't flap when the feature is off.
type TypesenseCheck struct{ Client *typesense.Client }

func (e TypesenseCheck) Name() string   { return "typesense" }
func (e TypesenseCheck) Critical() bool { return false }
func (e TypesenseCheck) Ping(ctx context.Context) error {
	if e.Client == nil {
		return nil
	}
	return e.Client.Ping(ctx)
}
