# Golang BE

**Production-grade, event-driven REST API** built with Go — the kind of system design you'd find behind products serving millions of users: polyglot persistence, event streaming with DLQ, full observability, and automated CI/CD.

[![CI](https://github.com/verdofanv/golang-be/actions/workflows/ci.yml/badge.svg)](https://github.com/verdofanv/golang-be/actions/workflows/ci.yml)
[![CodeQL](https://github.com/verdofanv/golang-be/actions/workflows/codeql.yml/badge.svg)](https://github.com/verdofanv/golang-be/actions/workflows/codeql.yml)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)]()

---

## Why this project exists

Portfolio backend yang meniru arsitektur tech-production kelas berat (Uber/Netflix-style) dalam satu repo yang runnable lokal:

- **Event-driven** — domain events mengalir lewat **Kafka**, dikonsumsi worker (consumer group) → audit trail immutable di **MongoDB**, dan fan-out real-time ke **WebSocket** clients
- **Reliability** — retry dengan exponential backoff + **Dead Letter Queue**, circuit breaker untuk Elasticsearch, graceful shutdown di semua proses
- **Observability** — **Prometheus** metrics (RED + business counters), **Grafana** dashboard, **OpenTelemetry** tracing ke Jaeger, structured logs dengan request ID
- **Polyglot persistence** — **PostgreSQL** (source of truth, versioned migrations), **Redis** (cache + rate limiting), **MongoDB** (audit/event store), **Elasticsearch** (full-text search)
- **Security** — JWT access/refresh + RBAC roles, Redis sliding-window rate limiter, OWASP security headers, API key gate, Trivy + CodeQL scanning di CI
- **Delivery** — GitHub Actions (lint → test → race → build → scan → GHCR), Kubernetes manifests + **Helm** chart dengan HPA

---

## Architecture

```mermaid
flowchart LR
    Client([Client / Mobile]) -->|HTTPS /api/v1| API[cmd/api<br/>Fiber HTTP]
    Client -->|WSS /ws/products| API

    API --> PG[(PostgreSQL<br/>source of truth)]
    API --> Redis[(Redis<br/>cache + rate limit)]
    API -->|search| ES[(Elasticsearch)]
    API -->|index docs| ES

    API -->|publish product.*| Kafka{{Kafka<br/>products.events}}

    Kafka -->|consumer group: notifier| API
    Kafka -->|consumer group: worker| Worker[cmd/worker]
    Worker -->|retry 3x, backoff| Worker
    Worker -->|exhausted| DLQ{{products.events.dlq}}
    Worker -->|immutable audit docs| Mongo[(MongoDB<br/>event_audit)]

    API --> Prom[/Prometheus scrape :8080\/metrics/]
    Worker --> Prom
    API -->|OTLP traces| Jaeger[/Jaeger/]
    Prom --> Grafana[/Grafana/]
```

**Request path** (`POST /api/v1/products`):
`recover → requestid → OTel tracing → Prometheus RED → security headers → CORS → gzip → timeout → request logger → apikey → rate limit → JWT auth → handler → service → PostgreSQL` → then **async**: cache write, ES index, Kafka publish.

**Event path**: `product.created` → Kafka topic → (1) `worker` group → MongoDB audit, (2) `notifier` group → WebSocket broadcast. Offsets committed **only after** side effects succeed (at-least-once), failures retry 3x with backoff → DLQ.

---

## Tech stack

| Concern            | Choice                                                             |
| ------------------ | ------------------------------------------------------------------ |
| Language           | Go **1.25**                                                        |
| HTTP               | Fiber v2                                                           |
| DI                 | Uber **fx**                                                        |
| RDBMS              | PostgreSQL 16 + GORM, migrations via **golang-migrate** (embedded) |
| Cache / rate limit | Redis 7 + `redis_rate` (Lua sliding window)                        |
| Event streaming    | **Apache Kafka** (KRaft) via `segmentio/kafka-go`                  |
| Document store     | **MongoDB 8** (audit trail)                                        |
| Search             | **Elasticsearch 8** + `gobreaker` circuit breaker                  |
| Real-time          | WebSocket (`gofiber/websocket`)                                    |
| Metrics            | Prometheus client (RED + business)                                 |
| Tracing            | OpenTelemetry OTLP → Jaeger                                        |
| Auth               | JWT (HS256) access/refresh + role claims (RBAC)                    |
| CI/CD              | GitHub Actions → GHCR, Trivy, CodeQL, Codecov, Dependabot          |
| Deploy             | Docker Compose (full stack), Kubernetes manifests, **Helm** chart  |

---

## Quick start

### 1. Configure

```bash
cp .env.example .env
```

### 2. Run everything (one command)

```bash
make docker-up
```

Boots: PostgreSQL, Redis, Kafka, MongoDB, Elasticsearch, Prometheus, Grafana, Jaeger, API, worker.

### 3. Explore

| Service    | URL                           | Credentials                  |
| ---------- | ----------------------------- | ---------------------------- |
| API        | http://localhost:8080         | header `apikey: dev-api-key` |
| Swagger UI | http://localhost:8080/docs    | —                            |
| Grafana    | http://localhost:3000         | `admin` / `admin`            |
| Prometheus | http://localhost:9090         | —                            |
| Jaeger     | http://localhost:16686        | enable `OTEL_ENABLED=true`   |
| Metrics    | http://localhost:8080/metrics | —                            |

Prefer running the apps on the host? `make infra-up` (infra only), then `make api` + `make worker` in two terminals.

### Admin account (seeded by migration 000002)

`admin@golang-be.dev` / `admin123` — role `admin` can delete **any** product (RBAC demo).

---

## API overview

All `/api/v1` routes require `apikey` header; protected routes also need `Authorization: Bearer <accessToken>`. Errors carry a stable machine code:

```json
{ "success": false, "message": "not found", "errorCode": "RESOURCE_NOT_FOUND" }
```

| Method   | Path                                   | Auth    | Notes                                    |
| -------- | -------------------------------------- | ------- | ---------------------------------------- |
| `GET`    | `/health/live`                         | —       | Liveness probe                           |
| `GET`    | `/health/ready`                        | —       | Readiness: pings PG/Redis/Mongo/ES       |
| `GET`    | `/metrics`                             | —       | Prometheus                               |
| `POST`   | `/api/v1/authentication/register`      | apikey  | Create account (role: user)              |
| `POST`   | `/api/v1/authentication/login`         | apikey  | Issue tokens                             |
| `POST`   | `/api/v1/authentication/refresh-token` | apikey  | Rotate tokens                            |
| `GET`    | `/api/v1/authentication/me`            | +bearer | Current user (incl. role)                |
| `GET`    | `/api/v1/products?cursor=&limit=`      | +bearer | **Cursor pagination** + first-page cache |
| `GET`    | `/api/v1/products/search?q=`           | +bearer | **Elasticsearch** full-text              |
| `POST`   | `/api/v1/products`                     | +bearer | Create → publishes Kafka event           |
| `GET`    | `/api/v1/products/:id`                 | +bearer | Detail (Redis cached)                    |
| `PUT`    | `/api/v1/products/:id`                 | +bearer | Update (owner) → event                   |
| `DELETE` | `/api/v1/products/:id`                 | +bearer | Delete (owner/**admin**) → event         |
| `GET`    | `/ws/products?token=`                  | JWT     | WebSocket real-time events               |

Full contract: [`docs/openapi.yaml`](docs/openapi.yaml)

### Cursor pagination

```
GET /api/v1/products?limit=2            → { data: [...], meta: { nextCursor: 42, hasMore: true } }
GET /api/v1/products?limit=2&cursor=42  → next page
```

Keyset pagination — stable under concurrent writes, O(log n) at any depth.

### Try the real-time flow

```bash
# 1. Login, copy accessToken
curl -s http://localhost:8080/api/v1/authentication/login \
  -H 'Content-Type: application/json' -H 'apikey: dev-api-key' \
  -d '{"email":"admin@golang-be.dev","password":"admin123"}'

# 2. Open a WebSocket (websocat or browser console)
websocat "ws://localhost:8080/ws/products?token=<accessToken>"

# 3. Create a product — the WS client receives the event instantly
curl -s http://localhost:8080/api/v1/products \
  -H 'Content-Type: application/json' -H 'apikey: dev-api-key' \
  -H "Authorization: Bearer <accessToken>" \
  -d '{"name":"Kopi Susu","description":"Iced","price":28000,"stock":10}'

# 4. See the audit trail in MongoDB
docker exec -it golang-be-mongodb-1 mongosh golang_be_audit --eval 'db.event_audit.find().pretty()'
```

---

## Project layout

```text
cmd/
  api/                 fx-wired HTTP service (composition root)
  worker/              fx-wired Kafka consumer → MongoDB audit + DLQ
internal/
  auth/                Register, login, refresh, me (+ role claims)
  product/             CRUD + cursor pagination + cache + events + search
  health/              /health/live + /health/ready (dependency probes)
  notify/              WebSocket hub + Kafka notifier consumer
  audit/               Worker: retry/backoff/DLQ processor + Mongo store
  config/              Validated env config (fail-fast on boot)
  domain/              Entities, sentinel errors, error codes
  middleware/          auth, rbac, apikey, ratelimit, security, timeout, tracing, logger
  metrics/             Prometheus RED + business metrics
  platform/            postgres, redis, kafka, mongo, elasticsearch, telemetry
  server/              Fiber app assembly (middleware order lives here)
migrations/            Versioned SQL (embedded into binaries)
pkg/response/          JSON envelope + validation binder
deploy → k8s/          Raw manifests (deployment, service, HPA, probes)
helm/golang-be/        Helm chart (api + worker + HPA)
docker/                Prometheus config + Grafana provisioning
docs/                  OpenAPI + Swagger UI
test/                  unit/ integration/ mocks/ testutil/
.github/               CI, CodeQL, release, Dependabot
```

---

## Testing & quality

```bash
make test               # unit + integration (no infra needed — mocks)
make test-race          # race detector
make test-cover         # coverage.html
make lint               # golangci-lint (gosec, gocritic, revive, ...)
make ci                 # the exact CI pipeline, locally
```

Testing philosophy: unit tests are black-box with in-memory repositories; integration tests boot the real Fiber app wired like production. Event publishing, pagination, RBAC, and the DLQ processor are all covered.

---

## CI/CD

| Workflow                                       | Trigger        | What it does                                                                                                             |
| ---------------------------------------------- | -------------- | ------------------------------------------------------------------------------------------------------------------------ |
| [`ci.yml`](.github/workflows/ci.yml)           | push/PR        | vet → golangci-lint → tests → race → coverage (Codecov) → build → Docker build → **Trivy scan** (fails on HIGH/CRITICAL) |
| [`codeql.yml`](.github/workflows/codeql.yml)   | push/PR/weekly | GitHub CodeQL security-and-quality analysis                                                                              |
| [`release.yml`](.github/workflows/release.yml) | main/tags      | Multi-arch-ready image build → push **GHCR** (`api`, `worker`) with semver/sha tags → GitHub Release with auto notes     |
| [`dependabot.yml`](.github/dependabot.yml)     | weekly         | Go modules, GitHub Actions, Docker base images                                                                           |

---

## Kubernetes & Helm

Raw manifests in [`k8s/`](k8s/): namespace, configmap, secret template, api/worker deployments (non-root, resource limits, liveness/readiness probes, preStop drain), ClusterIP service, and an **HPA** (2→10 pods on CPU 70%).

```bash
kubectl apply -f k8s/
# or
helm install golang-be ./helm/golang-be -n golang-be --create-namespace
```

Worker replicas double as a **Kafka consumer group** — scaling the deployment scales partition consumption for free.

---

## Design decisions worth reading

- **Migrations over AutoMigrate** — schema changes are reviewed, ordered, reversible SQL files embedded in the binary and applied on boot.
- **At-least-once + idempotent sink** — offsets commit only after Mongo write; `eventId` unique index makes redeliveries harmless.
- **Circuit breaker on search** — Elasticsearch can die without taking the API down (503 on `/search`, CRUD unaffected).
- **Fail-open rate limiter** — a Redis hiccup never blocks traffic; limits resume when Redis recovers.
- **Detached async side effects** — event publishing/indexing run in timeout-bounded goroutines; request latency never includes broker round-trips.
- **Config validation at boot** — a misconfigured process panics at startup with every invalid field listed, never at 3 AM in production.
