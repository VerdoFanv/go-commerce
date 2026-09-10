# Self-documenting Makefile — `make help` lists everything.
.DEFAULT_GOAL := help

APP_API    := api
APP_WORKER := worker

## ---- Development ----

.PHONY: api worker run
api: ## Run the HTTP API locally
	go run ./cmd/api

worker: ## Run the Kafka consumer worker locally
	go run ./cmd/worker

run: api ## Alias for `make api`

## ---- Dependencies ----

.PHONY: deps tidy
deps: ## Download and tidy Go modules
	go mod tidy

tidy: deps ## Alias for deps

## ---- Infrastructure ----

.PHONY: infra-up infra-down docker-up docker-down docker-build docker-logs obs-up obs-down
infra-up: ## Start core dependencies only (kafka, typesense) — light enough to run always
	# For k3s pods on the same host, set KAFKA_HOST_ADVERTISE to the LAN IP:
	#   KAFKA_HOST_ADVERTISE=192.168.0.155 make infra-up
	docker compose up -d kafka typesense

infra-down: ## Stop all containers (core + observability)
	docker compose --profile observability down

docker-build: ## Build api + worker images
	docker compose build api worker

docker-up: ## Build and start core stack (api, worker, kafka, typesense) — NOT observability
	docker compose up -d --build

docker-down: ## Stop the core stack
	docker compose down

docker-logs: ## Tail api + worker logs
	docker compose logs -f api worker

obs-up: ## Start observability plane (prometheus, grafana, jaeger, loki, promtail) — heavier, opt-in
	docker compose --profile observability up -d prometheus grafana jaeger loki promtail

obs-down: ## Stop observability plane only
	docker compose --profile observability stop prometheus grafana jaeger loki promtail

## ---- Database ----

.PHONY: migrate-install migrate-up migrate-down migrate-create
migrate-install: ## Install the golang-migrate CLI
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

migrate-up: ## Apply all migrations (requires DATABASE_URL)
	migrate -path migrations -database "$${DATABASE_URL}" up

migrate-down: ## Roll back one migration (requires DATABASE_URL)
	migrate -path migrations -database "$${DATABASE_URL}" down 1

migrate-create: ## Create a migration pair: make migrate-create NAME=add_orders
	migrate create -ext sql -dir migrations -seq $(NAME)

## ---- Testing ----

.PHONY: test test-unit test-integration test-race test-cover
test: ## Run unit + integration tests
	go test ./test/unit/... ./test/integration/...

test-unit: ## Run unit tests only
	go test ./test/unit/...

test-integration: ## Run integration tests only
	go test ./test/integration/...

test-race: ## Run tests with the race detector
	go test -race ./test/unit/... ./test/integration/...

test-cover: ## Run tests and export coverage.html
	go test -coverprofile=coverage.out ./test/unit/... ./test/integration/...
	go tool cover -html=coverage.out -o coverage.html

## ---- Load / benchmark (k6 via Docker) ----

K6_IMAGE := grafana/k6:0.54.0
K6_NET   := golang-be_default

.PHONY: load-smoke load-test bench bench-smoke bench-oversell bench-checkout
load-smoke: bench-smoke ## Alias — commerce smoke (order path)

load-test: ## Legacy product CRUD ramp (prefer make bench)
	docker run --rm -i --network $(K6_NET) -v $(PWD)/load:/scripts \
		-e BASE_URL=http://api:8080 -e API_KEY=$${API_KEY:-dev-api-key} \
		$(K6_IMAGE) run /scripts/load-test.js

bench: ## Full commerce benchmark matrix → load/results/ (see docs/BENCHMARK.md)
	BASE_URL=$${BASE_URL:-http://127.0.0.1:8080} API_KEY=$${API_KEY:-dev-api-key} ./scripts/bench.sh

bench-smoke: ## Gate: health + register + product + order
	BASE_URL=$${BASE_URL:-http://127.0.0.1:8080} API_KEY=$${API_KEY:-dev-api-key} BENCH_ONLY=smoke ./scripts/bench.sh

bench-oversell: ## Contention: N≫S buyers, assert no negative stock
	BASE_URL=$${BASE_URL:-http://127.0.0.1:8080} API_KEY=$${API_KEY:-dev-api-key} BENCH_ONLY=oversell ./scripts/bench.sh

bench-checkout: ## Sustained POST /orders (raise RATE_LIMIT_MAX first)
	BASE_URL=$${BASE_URL:-http://127.0.0.1:8080} API_KEY=$${API_KEY:-dev-api-key} BENCH_ONLY=checkout ./scripts/bench.sh

## ---- Quality ----

.PHONY: vet lint fmt ci
vet: ## go vet
	go vet ./...

lint: ## golangci-lint
	golangci-lint run --timeout=5m

fmt: ## gofmt + goimports over the tree
	gofmt -s -w .

ci: vet lint test ## The exact pipeline CI runs, locally
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/worker ./cmd/worker

## ---- Meta ----

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'
