// Package http groups everything that belongs to the API process delivery
// surface: Gin server wiring, middleware, health probes, feature handlers
// (auth/product/wishlist/lab), and real-time notify (WebSocket + Kafka notifier).
//
// Shared domain/config/platform live under internal/{domain,config,platform}.
// The Kafka worker lives under internal/worker.
package http
