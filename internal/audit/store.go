package audit

import (
	"context"

	"github.com/verdofanv/golang-be/internal/platform/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const collectionName = "event_audit"

// MongoStore persists audit records in MongoDB.
type MongoStore struct {
	client *mongo.Client
}

func NewMongoStore(client *mongo.Client) *MongoStore {
	return &MongoStore{client: client}
}

func (s *MongoStore) Insert(ctx context.Context, record Record) error {
	_, err := s.client.Collection(collectionName).InsertOne(ctx, record)
	return err
}

// EnsureIndexes creates the audit indexes idempotently (eventId unique →
// exactly-once audit semantics even under at-least-once delivery).
func (s *MongoStore) EnsureIndexes(ctx context.Context) error {
	indexModels := []mongodriver.IndexModel{
		{
			Keys:    bson.D{{Key: "eventId", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "type", Value: 1}, {Key: "processedAt", Value: -1}}},
	}
	_, err := s.client.Collection(collectionName).Indexes().CreateMany(ctx, indexModels)
	return err
}
