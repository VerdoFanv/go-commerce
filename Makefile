.PHONY: deps run api worker infra-up infra-down docker-up docker-down docker-build docker-logs tidy test

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
	go test ./...
