.PHONY: deps run api worker infra-up infra-down tidy test

deps:
	go mod tidy

tidy: deps

infra-up:
	docker compose up -d

infra-down:
	docker compose down

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

run: api

test:
	go test ./...
