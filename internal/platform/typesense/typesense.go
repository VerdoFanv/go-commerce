// Package typesense backs full-text product search. All calls are wrapped
// in a circuit breaker: when ES is down the API degrades (search returns 503)
// instead of cascading timeouts into the request path.
package typesense

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/typesense/typesense-go/v4/typesense"
	"github.com/typesense/typesense-go/v4/typesense/api"
	"github.com/typesense/typesense-go/v4/typesense/api/pointer"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/domain"
)

const indexName = "products"

type Client struct {
	ts      *typesense.Client
	breaker *gobreaker.CircuitBreaker[any]
}

func Connect(cfg config.Config) (*Client, error) {
	if cfg.TypesenseAddr == "" {
		return nil, nil // search disabled — callers must nil-check
	}

	tsClient := typesense.NewClient(
		typesense.WithServer(cfg.TypesenseAddr),
		typesense.WithAPIKey(cfg.TypesenseAPIKey),
	)

	c := &Client{ts: tsClient}
	// 5 consecutive failures open the circuit for 30s; half-open probes with 1 request.
	c.breaker = gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        "typesense",
		MaxRequests: 1,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			slog.Warn("circuit breaker state change", "name", name, "from", from.String(), "to", to.String())
		},
	})

	slog.Info("typesense connected", "addr", cfg.TypesenseAddr)

	// Schema only (fast). Document backfill from Postgres happens in API OnStart
	// via lab.BootstrapTypesense — SQL seed does not populate Typesense.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.BootstrapSchemas(ctx); err != nil {
		slog.Warn("typesense bootstrap schemas on connect", "err", err)
	}

	return c, nil
}

// BootstrapSchemas creates every search collection this process owns (idempotent).
// When you add a new index (e.g. users), add EnsureXCollection here and IndexX on
// the write path — do not rely on manual admin reindex for fresh labs.
func (c *Client) BootstrapSchemas(ctx context.Context) error {
	return c.EnsureProductsCollection(ctx)
}

// ProductDocument is the indexed shape (denormalized, search-optimized).
type ProductDocument struct {
	ID          string  `json:"id"`
	UserID      uint    `json:"userId"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
}

// IndexProduct upserts one product document. Failures are logged, never fatal:
// search is a read optimization, Postgres stays the source of truth.
func (c *Client) IndexProduct(ctx context.Context, p domain.Product) error {
	if err := c.EnsureProductsCollection(ctx); err != nil {
		return err
	}
	doc := ProductDocument{
		ID:          strconv.Itoa(int(p.ID)),
		UserID:      p.UserID,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
	}

	_, err := c.breaker.Execute(func() (any, error) {
		res, e := c.ts.Collection(indexName).Documents().Upsert(ctx, doc, &api.DocumentIndexParameters{})
		return res, e
	})
	return err
}

// DeleteProduct removes a document; missing docs are not an error.
func (c *Client) DeleteProduct(ctx context.Context, id uint) error {
	_, err := c.breaker.Execute(func() (any, error) {
		idStr := strconv.Itoa(int(id))

		res, e := c.ts.Collection(indexName).Document(idStr).Delete(ctx)
		return res, e
	})
	return err
}

// Search runs a full-text multi-match over name + description.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]ProductDocument, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	docs, err := c.searchOnce(ctx, query, limit)
	if err == nil {
		return docs, nil
	}
	// Fresh Typesense volume: collection missing → create schema and retry once.
	if isCollectionMissing(err) {
		if e := c.EnsureProductsCollection(ctx); e != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrUnavailable, e)
		}
		docs, err = c.searchOnce(ctx, query, limit)
		if err == nil {
			return docs, nil
		}
		// Empty collection after ensure → empty hits, not an error.
		if isCollectionMissing(err) {
			return []ProductDocument{}, nil
		}
	}
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return nil, domain.ErrUnavailable
	}
	return nil, fmt.Errorf("%w: %v", domain.ErrUnavailable, err)
}

func (c *Client) searchOnce(ctx context.Context, query string, limit int) ([]ProductDocument, error) {
	res, err := c.breaker.Execute(func() (any, error) {
		searchParams := &api.SearchCollectionParams{
			Q:       pointer.String(query),
			QueryBy: pointer.String("name,description"),
			PerPage: pointer.Int(limit),
		}
		result, err := c.ts.Collection(indexName).Documents().Search(ctx, searchParams)
		return result, err
	})
	if err != nil {
		return nil, err
	}

	searchRes := res.(*api.SearchResult)
	var docs []ProductDocument
	if searchRes != nil && searchRes.Hits != nil {
		for _, hit := range *searchRes.Hits {
			if hit.Document == nil {
				continue
			}
			docMap := *hit.Document
			docs = append(docs, ProductDocument{
				ID:          asString(docMap["id"]),
				UserID:      asUint(docMap["userId"]),
				Name:        asString(docMap["name"]),
				Description: asString(docMap["description"]),
				Price:       asFloat(docMap["price"]),
			})
		}
	}
	return docs, nil
}

func isCollectionMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "collection not found") ||
		(strings.Contains(msg, "not found") && strings.Contains(msg, "404"))
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return fmt.Sprint(v)
	}
}

func asUint(v any) uint {
	switch t := v.(type) {
	case float64:
		return uint(t)
	case int:
		return uint(t)
	case string:
		n, _ := strconv.Atoi(t)
		return uint(n)
	default:
		return 0
	}
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	default:
		return 0
	}
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.breaker.Execute(func() (any, error) {
		res, e := c.ts.Collections().Retrieve(ctx, &api.GetCollectionsParams{})
		return res, e
	})
	return err
}

// CollectionStats is a learning-friendly snapshot of the products index.
type CollectionStats struct {
	Name         string `json:"name"`
	NumDocuments int    `json:"numDocuments"`
	Exists       bool   `json:"exists"`
}

// Stats returns document count for the products collection (0 if missing).
func (c *Client) Stats(ctx context.Context) (CollectionStats, error) {
	res, err := c.breaker.Execute(func() (any, error) {
		col, e := c.ts.Collection(indexName).Retrieve(ctx)
		return col, e
	})
	if err != nil {
		// Collection may not exist yet — treat as empty rather than hard fail for lab UIs.
		return CollectionStats{Name: indexName, Exists: false}, nil
	}
	col := res.(*api.CollectionResponse)
	stats := CollectionStats{Name: indexName, Exists: true}
	if col.NumDocuments != nil {
		stats.NumDocuments = int(*col.NumDocuments)
	}
	return stats, nil
}

// EnsureProductsCollection creates the search schema if missing (idempotent).
func (c *Client) EnsureProductsCollection(ctx context.Context) error {
	_, err := c.breaker.Execute(func() (any, error) {
		schema := &api.CollectionSchema{
			Name: indexName,
			Fields: []api.Field{
				{Name: "id", Type: "string"},
				{Name: "userId", Type: "int32"},
				{Name: "name", Type: "string"},
				{Name: "description", Type: "string"},
				{Name: "price", Type: "float"},
			},
		}
		res, e := c.ts.Collections().Create(ctx, schema)
		return res, e
	})
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "already exists") || strings.Contains(msg, "conflict") {
			return nil
		}
		return err
	}
	return nil
}
