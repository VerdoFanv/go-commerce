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

.PHONY: infra-up infra-down docker-up docker-down docker-build docker-logs
infra-up: ## Start only infra containers (postgres, redis, kafka, mongo, es, observability)
	docker compose up -d postgres redis kafka mongodb elasticsearch prometheus grafana jaeger

infra-down: ## Stop all containers
	docker compose down

docker-build: ## Build api + worker images
	docker compose build api worker

docker-up: ## Build and start the full stack
	docker compose up -d --build

docker-down: ## Stop the full stack
	docker compose down

docker-logs: ## Tail api + worker logs
	docker compose logs -f api worker

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
