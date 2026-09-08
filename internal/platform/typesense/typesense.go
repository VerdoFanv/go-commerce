// Package typesense backs full-text product search. All calls are wrapped
// in a circuit breaker: when ES is down the API degrades (search returns 503)
// instead of cascading timeouts into the request path.
package typesense

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
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
	return c, nil
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
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, domain.ErrUnavailable
		}
		return nil, err
	}

	searchRes := res.(*api.SearchResult)
	var docs []ProductDocument
	
	if searchRes != nil && searchRes.Hits != nil {
		for _, hit := range *searchRes.Hits {
			if hit.Document != nil {
				docMap := *hit.Document
				docs = append(docs, ProductDocument{
					ID:          docMap["id"].(string),
					Name:        docMap["name"].(string),
					Description: docMap["description"].(string),
					Price:       docMap["price"].(float64), 
				})
			}
		}
	}
	return docs, nil
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.breaker.Execute(func () (any, error) {
		res, e := c.ts.Collections().Retrieve(ctx, &api.GetCollectionsParams{})
		return res, e
	})
	return err
}
