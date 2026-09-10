// Package mongo provides the document store used for the event-sourced audit
// trail: every consumed domain event lands here as an immutable document.
package mongo

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/verdofanv/golang-be/internal/config"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

type Client struct {
	client *mongo.Client
	db     *mongo.Database
}

func Connect(cfg config.Config) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}
	// Topology ping often succeeds even without credentials when Mongo has --auth.
	// Prove we can actually use MONGO_DB (listCollections needs auth).
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongo: %w", authHint(err, cfg.MongoURI))
	}
	if _, err := client.Database(cfg.MongoDB).ListCollectionNames(ctx, bson.D{}); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo auth/db %q: %w", cfg.MongoDB, authHint(err, cfg.MongoURI))
	}

	slog.Info("mongo connected", "db", cfg.MongoDB)
	return &Client{client: client, db: client.Database(cfg.MongoDB)}, nil
}

func authHint(err error, uri string) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unauthorized") || strings.Contains(msg, "auth") {
		noCreds := !strings.Contains(uri, "@")
		if noCreds {
			return fmt.Errorf("%w — MONGO_URI has no user:pass; set Secret MONGO_URI like mongodb://USER:PASS@HOST:27017/?authSource=admin (ConfigMap default is unauthenticated)", err)
		}
		return fmt.Errorf("%w — check MONGO_URI user/password and authSource=admin", err)
	}
	return err
}

// Collection returns a handle within the configured database.
func (c *Client) Collection(name string) *mongo.Collection {
	return c.db.Collection(name)
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx, readpref.Primary())
}

func (c *Client) Close(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}
