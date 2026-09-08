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

// ListRecent returns the newest audit documents (learning / admin read path).
func (s *MongoStore) ListRecent(ctx context.Context, limit int64) ([]Record, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	opts := options.Find().SetSort(bson.D{{Key: "processedAt", Value: -1}}).SetLimit(limit)
	cur, err := s.client.Collection(collectionName).Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []Record
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Record{}
	}
	return out, nil
}

// Count returns total audit documents.
func (s *MongoStore) Count(ctx context.Context) (int64, error) {
	return s.client.Collection(collectionName).CountDocuments(ctx, bson.D{})
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
