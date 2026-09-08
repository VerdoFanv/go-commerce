// Package worker groups everything that belongs to the worker process:
// Kafka consumers, audit persistence, retry/DLQ handling.
//
// Start from cmd/worker. Do not put Gin routes here.
package worker
