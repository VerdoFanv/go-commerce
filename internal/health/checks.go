package health

import (
	"context"

	"github.com/verdofanv/golang-be/internal/platform/elasticsearch"
	"github.com/verdofanv/golang-be/internal/platform/mongo"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"gorm.io/gorm"
)

// PostgresCheck pings via the underlying sql.DB.
type PostgresCheck struct{ DB *gorm.DB }

func (p PostgresCheck) Name() string { return "postgres" }
func (p PostgresCheck) Ping(ctx context.Context) error {
	sqlDB, err := p.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// RedisCheck pings the cache.
type RedisCheck struct{ Client *appredis.Client }

func (r RedisCheck) Name() string { return "redis" }
func (r RedisCheck) Ping(ctx context.Context) error {
	return r.Client.Raw().Ping(ctx).Err()
}

// MongoCheck pings the document store.
type MongoCheck struct{ Client *mongo.Client }

func (m MongoCheck) Name() string { return "mongo" }
func (m MongoCheck) Ping(ctx context.Context) error {
	return m.Client.Ping(ctx)
}

// ElasticsearchCheck pings the search cluster. A nil client (search disabled)
// reports healthy so probes don't flap when the feature is off.
type ElasticsearchCheck struct{ Client *elasticsearch.Client }

func (e ElasticsearchCheck) Name() string { return "elasticsearch" }
func (e ElasticsearchCheck) Ping(ctx context.Context) error {
	if e.Client == nil {
		return nil
	}
	return e.Client.Ping(ctx)
}
