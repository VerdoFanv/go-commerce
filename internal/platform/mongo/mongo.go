// Package mongo provides the document store used for the event-sourced audit
// trail: every consumed domain event lands here as an immutable document.
package mongo

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/verdofanv/golang-be/internal/config"
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
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	slog.Info("mongo connected", "db", cfg.MongoDB)
	return &Client{client: client, db: client.Database(cfg.MongoDB)}, nil
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
