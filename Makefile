.PHONY: deps run api worker infra-up infra-down docker-up docker-down docker-build docker-logs tidy test test-unit test-integration test-race test-cover lint vet ci

deps:
	go mod tidy

tidy: deps

infra-up:
	docker compose up -d postgres redis rabbitmq

infra-down:
	docker compose down

docker-build:
	docker compose build api worker

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f api worker

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

run: api

test:
	go test ./test/unit/... ./test/integration/...

test-unit:
	go test ./test/unit/...

test-integration:
	go test ./test/integration/...

test-race:
	go test -race ./test/unit/... ./test/integration/...

test-cover:
	go test -coverprofile=coverage.out ./test/unit/... ./test/integration/...
	go tool cover -html=coverage.out -o coverage.html

vet:
	go vet ./...

lint:
	golangci-lint run --timeout=5m

ci: vet lint test
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/api ./cmd/api
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/worker ./cmd/worker
