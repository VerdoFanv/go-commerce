// Package lab exposes learning-oriented HTTP endpoints that exercise each
// infrastructure dependency in a mid-tier polyglot backend. These routes are
// intentional teaching surfaces — not a substitute for product/wishlist APIs.
package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/http/product"
	"github.com/verdofanv/golang-be/internal/platform/kafka"
	"github.com/verdofanv/golang-be/internal/platform/outbox"
	appredis "github.com/verdofanv/golang-be/internal/platform/redis"
	"github.com/verdofanv/golang-be/internal/platform/typesense"
	"github.com/verdofanv/golang-be/internal/worker/audit"
	"gorm.io/gorm"
)

type EventPublisher interface {
	Publish(ctx context.Context, key string, event kafka.Event) error
}

type SearchEngine interface {
	IndexProduct(ctx context.Context, p domain.Product) error
	Search(ctx context.Context, query string, limit int) ([]typesense.ProductDocument, error)
	Stats(ctx context.Context) (typesense.CollectionStats, error)
	EnsureProductsCollection(ctx context.Context) error
}

// AuditReader is satisfied by *audit.MongoStore.
type AuditReader interface {
	ListRecent(ctx context.Context, limit int64) ([]audit.Record, error)
	Count(ctx context.Context) (int64, error)
}

type Service struct {
	db        *gorm.DB
	cache     *appredis.Client
	audit     AuditReader
	publisher EventPublisher
	search    SearchEngine
	relay     *outbox.Relay
	cfg       config.Config
}

func NewService(
	db *gorm.DB,
	cache *appredis.Client,
	auditStore AuditReader,
	publisher EventPublisher,
	search SearchEngine,
	relay *outbox.Relay,
	cfg config.Config,
) *Service {
	return &Service{
		db:        db,
		cache:     cache,
		audit:     auditStore,
		publisher: publisher,
		search:    search,
		relay:     relay,
		cfg:       cfg,
	}
}

type Overview struct {
	Postgres  PostgresSummary           `json:"postgres"`
	Redis     RedisSummary              `json:"redis"`
	Mongo     MongoSummary              `json:"mongo"`
	Kafka     KafkaSummary              `json:"kafka"`
	Typesense typesense.CollectionStats `json:"typesense"`
	Hint      string                    `json:"hint"`
}

type PostgresSummary struct {
	Users     int64 `json:"users"`
	Products  int64 `json:"products"`
	Wishlists int64 `json:"wishlists"`
	Orders    int64 `json:"orders"`
}

type RedisSummary struct {
	Connected bool     `json:"connected"`
	SampleKeys []string `json:"sampleKeys"`
}

type MongoSummary struct {
	AuditEvents int64 `json:"auditEvents"`
}

type KafkaSummary struct {
	Brokers       []string `json:"brokers"`
	TopicProducts string   `json:"topicProducts"`
	TopicDLQ      string   `json:"topicDlq"`
}

func (s *Service) Overview(ctx context.Context) (*Overview, error) {
	pg, err := s.PostgresSummary(ctx)
	if err != nil {
		return nil, err
	}
	redisSum := s.RedisSummary(ctx)
	mongoSum, err := s.MongoSummary(ctx)
	if err != nil {
		return nil, err
	}
	tsStats := typesense.CollectionStats{Name: "products"}
	if s.search != nil {
		if st, err := s.search.Stats(ctx); err == nil {
			tsStats = st
		}
	}
	return &Overview{
		Postgres:  *pg,
		Redis:     redisSum,
		Mongo:     *mongoSum,
		Kafka:     KafkaSummary{Brokers: s.cfg.KafkaBrokers, TopicProducts: s.cfg.KafkaTopicProducts, TopicDLQ: s.cfg.KafkaTopicDLQ},
		Typesense: tsStats,
		Hint:      "Gunakan endpoint /api/v1/lab/* untuk drill-down tiap infra. Lihat docs/PANDUAN-BELAJAR.md",
	}, nil
}

func (s *Service) PostgresSummary(ctx context.Context) (*PostgresSummary, error) {
	var users, products, wishlists, orders int64
	if err := s.db.WithContext(ctx).Table("users").Where("deleted_at IS NULL").Count(&users).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Table("products").Where("deleted_at IS NULL").Count(&products).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Table("wishlists").Count(&wishlists).Error; err != nil {
		return nil, err
	}
	_ = s.db.WithContext(ctx).Table("orders").Count(&orders).Error
	return &PostgresSummary{Users: users, Products: products, Wishlists: wishlists, Orders: orders}, nil
}

type SampleProduct struct {
	ID    uint    `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Stock int     `json:"stock"`
	Email string  `json:"ownerEmail"`
}

func (s *Service) PostgresSamples(ctx context.Context, limit int) ([]SampleProduct, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	type row struct {
		ID    uint
		Name  string
		Price float64
		Stock int
		Email string
	}
	var rows []row
	err := s.db.WithContext(ctx).Raw(`
		SELECT p.id, p.name, p.price, p.stock, u.email
		FROM products p
		JOIN users u ON u.id = p.user_id
		WHERE p.deleted_at IS NULL
		ORDER BY p.id DESC
		LIMIT ?
	`, limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]SampleProduct, 0, len(rows))
	for _, r := range rows {
		out = append(out, SampleProduct{ID: r.ID, Name: r.Name, Price: r.Price, Stock: r.Stock, Email: r.Email})
	}
	return out, nil
}

func (s *Service) RedisSummary(ctx context.Context) RedisSummary {
	sum := RedisSummary{Connected: s.cache != nil, SampleKeys: []string{}}
	if s.cache == nil {
		return sum
	}
	if err := s.cache.Raw().Ping(ctx).Err(); err != nil {
		sum.Connected = false
		return sum
	}
	keys, err := s.cache.Raw().Keys(ctx, "*").Result()
	if err != nil || keys == nil {
		return sum
	}
	if len(keys) > 20 {
		keys = keys[:20]
	}
	sum.SampleKeys = keys
	return sum
}

type CachePeek struct {
	Key     string          `json:"key"`
	Hit     bool            `json:"hit"`
	TTL     string          `json:"ttl"`
	Product *domain.Product `json:"product,omitempty"`
	Raw     string          `json:"raw,omitempty"`
	Lesson  string          `json:"lesson"`
}

func (s *Service) PeekProductCache(ctx context.Context, id uint) (*CachePeek, error) {
	key := fmt.Sprintf("product:%d", id)
	out := &CachePeek{
		Key:    key,
		Lesson: "GET product by id mengisi cache ini. DELETE/invalidate menghapusnya. Bandingkan hit=false vs hit=true.",
	}
	if s.cache == nil {
		return out, nil
	}
	ttl, _ := s.cache.Raw().TTL(ctx, key).Result()
	out.TTL = ttl.String()
	raw, err := s.cache.Raw().Get(ctx, key).Bytes()
	if err == redis.Nil {
		out.Hit = false
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Hit = true
	out.Raw = string(raw)
	var p domain.Product
	if json.Unmarshal(raw, &p) == nil {
		out.Product = &p
	}
	return out, nil
}

func (s *Service) InvalidateProductCache(ctx context.Context, id uint) (map[string]any, error) {
	key := fmt.Sprintf("product:%d", id)
	if s.cache == nil {
		return map[string]any{"deleted": 0, "key": key}, nil
	}
	n, err := s.cache.Raw().Del(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"deleted": n,
		"key":     key,
		"lesson":  "Setelah invalidate, GET /products/:id berikutnya harus miss cache lalu isi ulang dari Postgres.",
	}, nil
}

func (s *Service) MongoSummary(ctx context.Context) (*MongoSummary, error) {
	if s.audit == nil {
		return &MongoSummary{}, nil
	}
	n, err := s.audit.Count(ctx)
	if err != nil {
		return nil, err
	}
	return &MongoSummary{AuditEvents: n}, nil
}

func (s *Service) MongoEvents(ctx context.Context, limit int64) ([]audit.Record, error) {
	if s.audit == nil {
		return []audit.Record{}, nil
	}
	return s.audit.ListRecent(ctx, limit)
}

type KafkaPingResult struct {
	EventID string `json:"eventId"`
	Type    string `json:"type"`
	Topic   string `json:"topic"`
	Lesson  string `json:"lesson"`
}

func (s *Service) KafkaPing(ctx context.Context, userID uint) (*KafkaPingResult, error) {
	if s.publisher == nil {
		return nil, domain.ErrUnavailable
	}
	evt := kafka.NewEvent("lab.ping", map[string]any{
		"userId":  userID,
		"message": "hello from /api/v1/lab/kafka/ping",
		"at":      time.Now().UTC(),
	})
	key := strconv.FormatUint(uint64(userID), 10)
	if err := s.publisher.Publish(ctx, key, evt); err != nil {
		return nil, err
	}
	return &KafkaPingResult{
		EventID: evt.ID,
		Type:    evt.Type,
		Topic:   s.cfg.KafkaTopicProducts,
		Lesson:  "Cek kubectl logs deploy/worker — event audited type=lab.ping. Lalu GET /api/v1/lab/mongo/events.",
	}, nil
}

type TypesenseLab struct {
	Stats   typesense.CollectionStats   `json:"stats"`
	Query   string                      `json:"query,omitempty"`
	Results []typesense.ProductDocument `json:"results,omitempty"`
	Lesson  string                      `json:"lesson"`
}

func (s *Service) TypesenseExplore(ctx context.Context, query string) (*TypesenseLab, error) {
	if s.search == nil {
		return nil, domain.ErrUnavailable
	}
	stats, err := s.search.Stats(ctx)
	if err != nil {
		return nil, err
	}
	out := &TypesenseLab{
		Stats:  stats,
		Query:  query,
		Lesson: "API boot auto-reindex kalau index kosong. Manual: POST /api/v1/lab/typesense/reindex.",
	}
	if query != "" {
		docs, err := s.search.Search(ctx, query, 10)
		if err != nil {
			return nil, err
		}
		out.Results = docs
	}
	return out, nil
}

type ReindexResult struct {
	Indexed int    `json:"indexed"`
	Failed  int    `json:"failed"`
	Lesson  string `json:"lesson"`
}

func (s *Service) TypesenseReindex(ctx context.Context) (*ReindexResult, error) {
	if s.search == nil {
		return nil, domain.ErrUnavailable
	}
	if err := s.search.EnsureProductsCollection(ctx); err != nil {
		return nil, err
	}
	var models []product.ProductModel
	if err := s.db.WithContext(ctx).Where("deleted_at IS NULL").Find(&models).Error; err != nil {
		return nil, err
	}
	res := &ReindexResult{
		Lesson: "Postgres = source of truth. Typesense hanya index turunan — aman di-rebuild kapan saja.",
	}
	for _, m := range models {
		p := domain.Product{
			ID:          m.ID,
			UserID:      m.UserID,
			Name:        m.Name,
			Description: m.Description,
			Price:       m.Price,
			Stock:       m.Stock,
			CreatedAt:   m.CreatedAt,
			UpdatedAt:   m.UpdatedAt,
		}
		if err := s.search.IndexProduct(ctx, p); err != nil {
			res.Failed++
			continue
		}
		res.Indexed++
	}
	return res, nil
}

// BootstrapTypesense ensures the products schema exists and backfills from
// Postgres when the index is empty (typical after fresh Typesense volume + SQL seed).
func (s *Service) BootstrapTypesense(ctx context.Context) error {
	if s.search == nil {
		return nil
	}
	if err := s.search.EnsureProductsCollection(ctx); err != nil {
		return err
	}
	stats, err := s.search.Stats(ctx)
	if err != nil {
		return err
	}
	if stats.NumDocuments > 0 {
		slog.Info("typesense bootstrap skip reindex", "numDocuments", stats.NumDocuments)
		return nil
	}
	var n int64
	if err := s.db.WithContext(ctx).Model(&product.ProductModel{}).Where("deleted_at IS NULL").Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		slog.Info("typesense bootstrap: no products in postgres yet")
		return nil
	}
	res, err := s.TypesenseReindex(ctx)
	if err != nil {
		return err
	}
	slog.Info("typesense bootstrap reindexed", "indexed", res.Indexed, "failed", res.Failed)
	return nil
}

// FailureMatrix documents expected resilience behaviour for the commerce lab.
func (s *Service) FailureMatrix(_ context.Context) map[string]any {
	return map[string]any{
		"lesson": "Replicate large-system failure modes without breaking order durability.",
		"matrix": []map[string]string{
			{"failure": "PostgreSQL down", "expected": "API degraded/unavailable; /health/ready fails"},
			{"failure": "Redis down", "expected": "cache bypass — product/wishlist still work from Postgres"},
			{"failure": "Kafka down", "expected": "POST /orders still 201; rows sit in outbox_events until relay succeeds"},
			{"failure": "Typesense down", "expected": "search degraded (circuit breaker); CRUD unaffected"},
			{"failure": "Worker crash", "expected": "Kafka redelivery; payment/inventory/audit handlers are idempotent"},
			{"failure": "Duplicate event", "expected": "no double charge / no double stock release (status + unique keys)"},
			{"failure": "Network timeout", "expected": "relay retries; request timeout middleware bounds HTTP"},
			{"failure": "Consumer overload", "expected": "lag grows; no data loss (at-least-once + idempotent)"},
			{"failure": "Pod killed", "expected": "k3s redirects traffic; graceful Shutdown drains in-flight"},
			{"failure": "DB slow", "expected": "REQUEST_TIMEOUT aborts slow handlers"},
		},
		"triggers": map[string]string{
			"pauseRelay":   "POST /api/v1/lab/outbox/pause — simulate Kafka-unavailable publish path",
			"resumeRelay":  "POST /api/v1/lab/outbox/resume",
			"pendingOutbox": "GET /api/v1/lab/outbox/pending",
			"relayOnce":    "POST /api/v1/lab/outbox/relay-once",
			"payFail":      "POST /api/v1/orders/:id/pay {\"outcome\":\"fail\"}",
		},
	}
}

func (s *Service) OutboxPending(ctx context.Context, limit int) (map[string]any, error) {
	if s.relay == nil {
		return nil, domain.ErrUnavailable
	}
	rows, err := s.relay.ListPending(ctx, limit)
	if err != nil {
		return nil, err
	}
	n, err := s.relay.PendingCount(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"pendingCount": n,
		"paused":       s.relay.Paused(),
		"items":        rows,
		"lesson":       "Unpublished outbox rows prove the request stayed safe while Kafka/relay was unavailable.",
	}, nil
}

func (s *Service) OutboxRelayOnce(ctx context.Context) (map[string]any, error) {
	if s.relay == nil {
		return nil, domain.ErrUnavailable
	}
	n, err := s.relay.RelayOnce(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"published": n, "paused": s.relay.Paused()}, nil
}

func (s *Service) OutboxSetPaused(paused bool) map[string]any {
	if s.relay == nil {
		return map[string]any{"ok": false, "error": "relay unavailable"}
	}
	s.relay.SetPaused(paused)
	return map[string]any{"ok": true, "paused": s.relay.Paused()}
}

