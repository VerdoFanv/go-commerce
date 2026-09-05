// Package elasticsearch backs full-text product search. All calls are wrapped
// in a circuit breaker: when ES is down the API degrades (search returns 503)
// instead of cascading timeouts into the request path.
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/sony/gobreaker/v2"
	"github.com/verdofanv/golang-be/internal/config"
	"github.com/verdofanv/golang-be/internal/domain"
)

const indexName = "products"

type Client struct {
	es      *elasticsearch.Client
	breaker *gobreaker.CircuitBreaker[[]byte]
}

func Connect(cfg config.Config) (*Client, error) {
	if cfg.ElasticsearchAddr == "" {
		return nil, nil // search disabled — callers must nil-check
	}

	es, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{cfg.ElasticsearchAddr},
	})
	if err != nil {
		return nil, fmt.Errorf("connect elasticsearch: %w", err)
	}

	c := &Client{es: es}
	// 5 consecutive failures open the circuit for 30s; half-open probes with 1 request.
	c.breaker = gobreaker.NewCircuitBreaker[[]byte](gobreaker.Settings{
		Name:        "elasticsearch",
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

	slog.Info("elasticsearch connected", "addr", cfg.ElasticsearchAddr)
	return c, nil
}

// ProductDocument is the indexed shape (denormalized, search-optimized).
type ProductDocument struct {
	ID          uint    `json:"id"`
	UserID      uint    `json:"userId"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
}

// IndexProduct upserts one product document. Failures are logged, never fatal:
// search is a read optimization, Postgres stays the source of truth.
func (c *Client) IndexProduct(ctx context.Context, p domain.Product) error {
	doc := ProductDocument{
		ID:          p.ID,
		UserID:      p.UserID,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal product doc: %w", err)
	}

	_, err = c.breaker.Execute(func() ([]byte, error) {
		res, err := c.es.Index(
			indexName,
			bytes.NewReader(body),
			c.es.Index.WithContext(ctx),
			c.es.Index.WithDocumentID(fmt.Sprintf("%d", p.ID)),
			c.es.Index.WithRefresh("wait_for"),
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = res.Body.Close() }()
		if res.IsError() {
			raw, _ := io.ReadAll(res.Body)
			return nil, fmt.Errorf("index doc: status=%s body=%s", res.Status(), raw)
		}
		return nil, nil
	})
	return err
}

// DeleteProduct removes a document; missing docs are not an error.
func (c *Client) DeleteProduct(ctx context.Context, id uint) error {
	_, err := c.breaker.Execute(func() ([]byte, error) {
		res, err := c.es.Delete(
			indexName,
			fmt.Sprintf("%d", id),
			c.es.Delete.WithContext(ctx),
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = res.Body.Close() }()
		if res.IsError() && res.StatusCode != 404 {
			raw, _ := io.ReadAll(res.Body)
			return nil, fmt.Errorf("delete doc: status=%s body=%s", res.Status(), raw)
		}
		return nil, nil
	})
	return err
}

// Search runs a full-text multi-match over name + description.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]ProductDocument, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	esQuery := map[string]any{
		"size": limit,
		"query": map[string]any{
			"multi_match": map[string]any{
				"query":     query,
				"fields":    []string{"name^2", "description"},
				"fuzziness": "AUTO",
			},
		},
	}
	body, err := json.Marshal(esQuery)
	if err != nil {
		return nil, fmt.Errorf("marshal search query: %w", err)
	}

	raw, err := c.breaker.Execute(func() ([]byte, error) {
		res, err := c.es.Search(
			c.es.Search.WithContext(ctx),
			c.es.Search.WithIndex(indexName),
			c.es.Search.WithBody(bytes.NewReader(body)),
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = res.Body.Close() }()
		if res.IsError() {
			b, _ := io.ReadAll(res.Body)
			return nil, fmt.Errorf("search: status=%s body=%s", res.Status(), b)
		}
		return io.ReadAll(res.Body)
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) {
			return nil, domain.ErrUnavailable
		}
		return nil, err
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				Source ProductDocument `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	docs := make([]ProductDocument, 0, len(parsed.Hits.Hits))
	for _, hit := range parsed.Hits.Hits {
		docs = append(docs, hit.Source)
	}
	return docs, nil
}

func (c *Client) Ping(ctx context.Context) error {
	res, err := c.es.Ping(c.es.Ping.WithContext(ctx))
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.IsError() {
		return fmt.Errorf("ping: %s", res.Status())
	}
	return nil
}
