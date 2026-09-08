# Golang BE

**Production-grade, event-driven REST API** built with Go — a personal portfolio backend that mirrors mid-to-large system design: polyglot persistence, event streaming with DLQ, full observability, and flexible delivery (self-deploy **or** GitLab CI/CD).

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Gin](https://img.shields.io/badge/HTTP-Gin-00ADD8)](https://gin-gonic.com/)
[![GitLab CI](https://img.shields.io/badge/CI-GitLab-FC6D26?logo=gitlab)](docs/GITLAB-SETUP.md)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)]()

---

## Portfolio goal

This repo is a **personal portfolio** backend: not a thin CRUD demo, but architecture decisions you would discuss in interviews for mid/senior backend roles (event-driven flows, workers, Kubernetes rollouts, observability).

It is **right-sized** for a single Ubuntu box (k3s + Docker Compose) — credible production patterns without needing a 100-node cluster.

**Guides:** [Learning guide (ID)](docs/PANDUAN-BELAJAR.md) · [GitLab setup](docs/GITLAB-SETUP.md) · [Public IP / domain](docs/PUBLIC-ACCESS.md)

---

## Why this stack

- **Event-driven** — Kafka domain events → worker audit in MongoDB + WebSocket fan-out
- **Reliability** — retries, Dead Letter Queue, circuit breaker on search, graceful shutdown
- **Observability** — Prometheus / Grafana / OpenTelemetry / Loki (opt-in)
- **Polyglot persistence** — PostgreSQL + Redis + MongoDB + Typesense
- **Security** — JWT + RBAC, rate limiting, API key gate, OWASP headers; secrets from env / K8s Secret
- **Delivery** — GitLab CI **and/or** manual deploy to k3s with zero-downtime rolling updates

---

## Positive impact of this stack

| Choice | What you gain |
| ------ | ------------- |
| **PostgreSQL as source of truth** | Strong consistency for users/products; clear ownership of business data |
| **Redis** | Lower latency on hot reads; built-in sliding-window rate limiting across replicas |
| **Kafka + worker** | Decoupled side effects, durable event log, consumer groups, DLQ for poison messages |
| **MongoDB audit** | Immutable event history for debugging, compliance demos, and “what happened” timelines |
| **Typesense** | Production-style full-text search without overloading Postgres `LIKE` scans |
| **Gin + clean architecture** | Industry-standard Go HTTP stack; handlers stay thin; services are unit-testable |
| **SQL migrations (golang-migrate)** | Reviewable, ordered, reversible schema changes — same process local → server |
| **k3s (app tier)** | Self-healing pods, rolling updates, HPA, readiness-based traffic — real deploy muscle |
| **GitLab CI + Container Registry** | Repeatable lint/test/build/scan/push; optional automated release |
| **Observability plane** | You can *show* RED metrics, traces, and logs — not only claim them |

**Interview angle:** you can explain *why* each store exists and how traffic flows through the system — the positive signal this portfolio is built for.

---

## Migrations: why `.sql` files?

**Yes — this is production-grade practice.**

- `golang-migrate` + `migrations/00000N_*.up.sql` / `.down.sql` is the schema source of truth
- Reviewable in MRs, applied on API boot (`database.Migrate`), reversible in controlled environments
- Prefer this over GORM `AutoMigrate` in production (harder to audit and roll back)

Other solid tools (not used here): Atlas, Flyway, Liquibase. AutoMigrate is fine only for throwaway prototypes.

---

## Secrets & config

| Source | Contents | Commit? |
| ------ | -------- | ------- |
| `.env` (from `.env.example`) | Local development | No (gitignored) |
| `k8s/secret.yaml` (from `secret.example.yaml`) | Runtime secrets on k3s | No (gitignored) |
| `k8s/configmap.yaml` | Non-secret config (hosts, ports, topic names) | Yes |
| Go defaults in `config.Load` | Local boot fallbacks only | Yes — never real production passwords |

Put real credentials in `.env` / `k8s/secret.yaml` / your password manager — not in git.

---

## Architecture

```mermaid
flowchart LR
    Client([Client / Mobile]) -->|HTTPS /api/v1| API[cmd/api<br/>Gin HTTP]
    Client -->|WSS /ws/products| API

    API --> PG[(PostgreSQL<br/>source of truth)]
    API --> Redis[(Redis<br/>cache + rate limit)]
    API -->|search| TS[(Typesense)]
    API -->|index docs| TS

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
`recover → requestid → OTel → Prometheus RED → security → CORS → timeout → logger → apikey → rate limit → JWT → handler → service → PostgreSQL` → async: cache, Typesense, Kafka.

---

## Tech stack

| Concern | Choice |
| ------- | ------ |
| Language | Go **1.25** |
| HTTP | **Gin** |
| DI | Uber **fx** |
| RDBMS | PostgreSQL + GORM + **golang-migrate** (SQL files) |
| Cache / rate limit | Redis + `redis_rate` |
| Events | Kafka (KRaft) + `segmentio/kafka-go` |
| Audit | MongoDB |
| Search | Typesense + circuit breaker |
| Real-time | WebSocket (`gorilla/websocket`) |
| CI/CD | **GitLab CI** → Container Registry + Trivy (optional auto-deploy) |
| Deploy | Docker Compose (data plane) + **k3s** / Helm (app tier) |

---

## Two delivery modes

| Mode | When to use | How |
| ---- | ----------- | --- |
| **A — Self-deploy** | Day-to-day lab, offline, fastest iteration | Build on the server → `k3s ctr images import` → `kubectl rollout` |
| **B — GitLab CI/CD** | Clean releases, tags, portfolio “pipeline” story | Push → pipeline builds/scans → push images → optional **manual** deploy job |

Both are first-class. Details: [`docs/GITLAB-SETUP.md`](docs/GITLAB-SETUP.md).

### Zero-downtime rollout (k3s)

API deployment uses `replicas: 2`, `maxUnavailable: 0`, readiness `/health/ready`, `preStop` drain, and graceful HTTP shutdown. New pods must be Ready before old pods terminate.

```bash
# Mode A (self-deploy on the Ubuntu box)
docker build --target api -t golang-be-api:local .
docker save golang-be-api:local -o /tmp/api.tar && sudo k3s ctr images import /tmp/api.tar
# repeat for worker
sudo k3s kubectl -n golang-be apply -f k8s/api-deployment.yaml
sudo k3s kubectl -n golang-be rollout restart deploy/api deploy/worker
sudo k3s kubectl -n golang-be rollout status deploy/api
```

---

## Quick start

### 1. Configure

```bash
cp .env.example .env
# Fill DB_*/MONGO_*/API_KEY/JWT_SECRET from your local infra — never commit .env
```

### 2. Run the core stack

```bash
make docker-up      # api, worker, kafka, typesense
```

Postgres/Redis/MongoDB are expected on `shared-net`. Observability is opt-in:

```bash
make obs-up
make obs-down
```

### 3. Explore

| Service | URL | Credentials |
| ------- | --- | ----------- |
| API | http://localhost:8080 | header `apikey: dev-api-key` |
| Swagger UI | http://localhost:8080/docs | — |
| Grafana | http://localhost:3000 | `admin` / `admin` |
| Prometheus | http://localhost:9090 | — |
| Jaeger | http://localhost:16686 | set `OTEL_ENABLED=true` |
| Metrics | http://localhost:8080/metrics | — |

Prefer host processes? `make infra-up`, then `make api` + `make worker`.

### Admin account (migration 000002)

`admin@golang-be.dev` / `admin123` — role `admin` can delete any product (RBAC demo).

---

## API overview

All `/api/v1` routes require `apikey`; protected routes also need `Authorization: Bearer <accessToken>`.

```json
{ "success": false, "message": "not found", "errorCode": "RESOURCE_NOT_FOUND" }
```

| Method | Path | Auth | Notes |
| ------ | ---- | ---- | ----- |
| `GET` | `/health/live` | — | Liveness |
| `GET` | `/health/ready` | — | Readiness (PG/Redis/Mongo/Typesense) |
| `GET` | `/metrics` | — | Prometheus |
| `POST` | `/api/v1/authentication/register` | apikey | Create account |
| `POST` | `/api/v1/authentication/login` | apikey | Issue tokens |
| `POST` | `/api/v1/authentication/refresh-token` | apikey | Rotate tokens |
| `GET` | `/api/v1/authentication/me` | +bearer | Current user |
| `GET` | `/api/v1/products` | +bearer | Cursor pagination + cache |
| `GET` | `/api/v1/products/search?q=` | +bearer | Typesense full-text |
| `POST` | `/api/v1/products` | +bearer | Create → Kafka event |
| `GET`/`PUT`/`DELETE` | `/api/v1/products/:id` | +bearer | Detail / update / delete |
| `GET`/`POST`/`DELETE` | `/api/v1/wishlists` | +bearer | Wishlist (+ Redis count) |
| `GET`/`POST` | `/api/v1/lab/*` | +bearer | Infra learning endpoints |
| `GET` | `/ws/products?token=` | JWT | Real-time events |

Full contract: [`docs/openapi.yaml`](docs/openapi.yaml)

---

## Project layout

```text
cmd/
  api/                      HTTP API process (Gin + fx)
  worker/                   Kafka worker process (fx)

internal/
  http/                     ★ everything for the API process
    server/                 Gin engine wiring
    middleware/             auth, apikey, ratelimit, ...
    health/                 /health/live|ready
    auth|product|wishlist|lab/
    notify/                 WebSocket + Kafka notifier
  worker/                   ★ everything for the worker process
    audit/                  consume → Mongo audit + DLQ
  domain/                   shared entities + errors
  config/                   shared env config
  platform/                 shared infra adapters
  metrics/                  shared Prometheus metrics

migrations/                 versioned SQL
k8s/ helm/                  deploy
.gitlab-ci.yml              GitLab CI (self-deploy or pipeline)
docs/                       OpenAPI, guides
test/                       unit / integration / mocks
```

---

## Testing & quality

```bash
make test               # unit + integration (mocks — no infra)
make test-race
make test-cover
make lint
make ci                 # local CI-equivalent checks
```

---

## Load testing

```bash
make docker-up
make load-smoke
make load-test
```

---

## CI/CD (GitLab)

See [`docs/GITLAB-SETUP.md`](docs/GITLAB-SETUP.md) and [`.gitlab-ci.yml`](.gitlab-ci.yml).

| Stage | Purpose |
| ----- | ------- |
| lint / test / build | Every branch & MR |
| docker | Image build + Trivy |
| release | Push to GitLab Container Registry (`main` / tags) |
| deploy | **Manual** job — pull images on the server and rollout (Mode B) |

---

## Kubernetes (k3s)

The cluster runs only the **stateless app tier** (`api` + `worker`). Data services stay on Docker Compose on the same host.

Public / domain access later: [`docs/PUBLIC-ACCESS.md`](docs/PUBLIC-ACCESS.md).

```bash
kubectl apply -f k8s/
# or
helm install golang-be ./helm/golang-be -n golang-be --create-namespace \
  --set config.hostIP=192.168.0.155
```

---

## Design decisions worth reading

- **Migrations over AutoMigrate** — reviewed, ordered, reversible SQL
- **At-least-once + idempotent sink** — commit after Mongo write; unique `eventId`
- **Circuit breaker on search** — Typesense outage does not kill CRUD
- **Fail-open rate limiter** — Redis blip never blocks all traffic
- **Async side effects** — Kafka/Typesense do not inflate HTTP latency
- **Config validation at boot** — misconfig fails fast, including production secret guards
