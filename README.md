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

Beyond infra, it also labs **commerce reliability** problems found in large systems: order state machine, inventory hold, payment choreography, transactional outbox, consumer inbox, stock ledger, and a live failure matrix.

**Guides:** [Lab deploy / teardown](#lab-deploy-mode-a--commands) · [Benchmark / capacity](docs/BENCHMARK.md) · [Failure runbook](docs/FAILURE-RUNBOOK.md) · [Postman](docs/postman/) · [OpenAPI](docs/openapi.yaml) · [Code walkthrough (Indonesian)](docs/belajar/README.md) · [Ops / lab (Indonesian)](docs/PANDUAN-BELAJAR.md) · [GitLab setup](docs/GITLAB-SETUP.md) · [Public IP / domain](docs/PUBLIC-ACCESS.md)

---

## Why this stack

- **Event-driven** — Kafka domain events → worker (payment / inventory / audit) + WebSocket fan-out
- **Reliability** — transactional outbox, consumer inbox, retries, DLQ, search circuit breaker, graceful shutdown
- **Commerce lab** — orders, stock holds, payment simulator, idempotency keys, failure-matrix endpoints
- **Observability** — Prometheus / Grafana / OpenTelemetry / Loki (opt-in)
- **Polyglot persistence** — PostgreSQL + Redis + MongoDB + Typesense
- **Security** — JWT + RBAC (lab/fulfill admin), refresh rotation + logout, rate limiting (auth fail-closed), API key (constant-time), OWASP headers, CORS allowlist, optional HSTS; secrets from env / K8s Secret
- **Delivery** — GitLab CI **and/or** manual deploy to k3s with zero-downtime rolling updates
- **Deploy habit** — change code locally → commit/push → on the server only `git pull` + rebuild/rollout (do not edit code over SSH)

---

## Positive impact of this stack

| Choice                              | What you gain                                                                                   |
| ----------------------------------- | ----------------------------------------------------------------------------------------------- |
| **PostgreSQL as source of truth**   | Strong consistency for users/products/**orders**; clear ownership of business data              |
| **Transactional outbox**            | Order writes stay correct when Kafka is down — events publish later, not lost                   |
| **Consumer inbox + stock ledger**   | Duplicate Kafka deliveries do not double-charge or corrupt inventory; stock moves are auditable |
| **Redis**                           | Lower latency on hot reads; built-in sliding-window rate limiting across replicas               |
| **Kafka + worker**                  | Decoupled side effects, durable event log, consumer groups, DLQ for poison messages             |
| **MongoDB audit**                   | Immutable event history for debugging, compliance demos, and “what happened” timelines          |
| **Typesense**                       | Production-style full-text search without overloading Postgres `LIKE` scans                     |
| **Gin + clean architecture**        | Industry-standard Go HTTP stack; handlers stay thin; services are unit-testable                 |
| **SQL migrations (golang-migrate)** | Reviewable, ordered, reversible schema changes — same process local → server                    |
| **k3s (app tier)**                  | Self-healing pods, rolling updates, HPA, readiness-based traffic — real deploy muscle           |
| **GitLab CI + Container Registry**  | Repeatable lint/test/build/scan/push; optional automated release                                |
| **Observability plane**             | You can _show_ RED metrics, traces, outbox lag, and logs — not only claim them                  |

**Interview angle:** you can explain _why_ each store exists, how an order survives broker outages, and how traffic flows — the positive signal this portfolio is built for.

---

## Migrations: why `.sql` files?

**Yes — this is production-grade practice.**

- `golang-migrate` + `migrations/00000N_*.up.sql` / `.down.sql` is the schema source of truth
- Reviewable in MRs, applied on API boot (`database.Migrate`), reversible in controlled environments
- Prefer this over GORM `AutoMigrate` in production (harder to audit and roll back)

Other solid tools (not used here): Atlas, Flyway, Liquibase. AutoMigrate is fine only for throwaway prototypes.

---

## Secrets & config

| Source                                         | Contents                                      | Commit?                               |
| ---------------------------------------------- | --------------------------------------------- | ------------------------------------- |
| `.env` (from `.env.example`)                   | Local development                             | No (gitignored)                       |
| `k8s/secret.yaml` (from `secret.example.yaml`) | Runtime secrets on k3s                        | No (gitignored)                       |
| `k8s/configmap.yaml`                           | Non-secret config (hosts, ports, topic names) | Yes                                   |
| Go defaults in `config.Load`                   | Local boot fallbacks only                     | Yes — never real production passwords |

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

    API -->|outbox relay + product publish| Kafka{{Kafka<br/>products.events}}

    Kafka -->|consumer group: notifier| API
    Kafka -->|consumer group: worker| Worker[cmd/worker<br/>dispatch]
    Worker -->|payment / inventory| PG
    Worker -->|retry 3x, backoff| Worker
    Worker -->|exhausted| DLQ{{products.events.dlq}}
    Worker -->|immutable audit docs| Mongo[(MongoDB<br/>event_audit)]

    API --> Prom[/Prometheus scrape :8080\/metrics/]
    Worker --> Prom
    API -->|OTLP traces| Jaeger[/Jaeger/]
    Prom --> Grafana[/Grafana/]
```

**Two publish paths (important for interviews):**

| Path                  | How events leave the API                                                              | When Kafka is down                                                            |
| --------------------- | ------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| **Product CRUD**      | Fire-and-forget `publishAsync` after DB commit                                        | Event may be lost (demo of the dual-write problem)                            |
| **Orders (commerce)** | Same Postgres TX writes order + `outbox_events`; API **outbox relay** publishes later | `POST /orders` still **201** — pending rows visible via `/lab/outbox/pending` |

**Request path** (`POST /api/v1/products`):
`recover → requestid → OTel → Prometheus RED → security → CORS → timeout → logger → apikey → rate limit → JWT → handler → service → PostgreSQL` → async: cache, Typesense, Kafka.

**Order path** (`POST /api/v1/orders` + `Idempotency-Key`):
same middleware → hold stock + order + outbox in **one TX** → relay → worker payment → inventory commit → audit Mongo.

Deep dive (Indonesian): [`docs/belajar/09-commerce-reliability.md`](docs/belajar/09-commerce-reliability.md).

---

## Tech stack

| Concern            | Choice                                                               |
| ------------------ | -------------------------------------------------------------------- |
| Language           | Go **1.25**                                                          |
| HTTP               | **Gin**                                                              |
| DI                 | Uber **fx**                                                          |
| RDBMS              | PostgreSQL + GORM + **golang-migrate** (SQL files)                   |
| Cache / rate limit | Redis + `redis_rate`                                                 |
| Events             | Kafka (KRaft) + `segmentio/kafka-go` + **transactional outbox**      |
| Reliability        | Consumer **inbox**, stock **ledger**, DLQ, Typesense circuit breaker |
| Audit              | MongoDB                                                              |
| Search             | Typesense + circuit breaker                                          |
| Real-time          | WebSocket (`gorilla/websocket`)                                      |
| Commerce           | Orders / inventory reservations / payment simulator / fulfill        |
| CI/CD              | **GitLab CI** → Container Registry + Trivy (optional auto-deploy)    |
| Deploy             | Docker Compose (data plane) + **k3s** / Helm (app tier)              |

---

## Two delivery modes

| Mode                      | When to use                         | How                                                   |
| ------------------------- | ----------------------------------- | ----------------------------------------------------- |
| **A — Self-deploy (lab)** | Day-to-day on one Ubuntu box        | Compose = Kafka/Typesense; **k3s** = api + worker     |
| **B — GitLab CI/CD**      | Clean releases / portfolio pipeline | Push → build/scan → registry → optional manual deploy |

Both are first-class. Ops detail (Indonesian): [`docs/PANDUAN-BELAJAR.md`](docs/PANDUAN-BELAJAR.md) · GitLab: [`docs/GITLAB-SETUP.md`](docs/GITLAB-SETUP.md).

**Split that matters:** do **not** run api/worker in Compose when using k3s. Compose owns the data plane; k3s owns the app.

---

## Lab deploy (Mode A) — commands

### One-time / first boot

```bash
# 1) shared data (Postgres / Redis / Mongo) — already on shared-net
cd ~/projects/infra-db && docker compose up -d

# 2) k3s
curl -sfL https://get.k3s.io | sh -   # skip if already installed
sudo systemctl enable --now k3s
sudo k3s kubectl get nodes

# 3) app repo + secrets (never commit secret.yaml / .env)
cd ~/projects/golang-be
git pull
cp .env.example .env                  # adjust if needed
cp k8s/secret.example.yaml k8s/secret.yaml
# fill in API_KEY, JWT_SECRET, DB_USER/PASSWORD, TYPESENSE_API_KEY,
# MONGO_URI=mongodb://admin:...@192.168.0.155:27017/?authSource=admin
```

### Every deploy / code update

Workflow: **edit locally → commit → push → on the server `git pull` → build & rollout** (do not edit Go over SSH).

```bash
cd ~/projects/golang-be
git pull

# shortcut (recommended)
HOST_IP=192.168.0.155 ./scripts/deploy-k3s-lab.sh
```

That script runs: Kafka+Typesense → build `golang-be-api:local` / `golang-be-worker:local` → `k3s ctr images import` → apply ConfigMap/Secret/manifests → rollout.

### Restart the app only (no rebuild)

```bash
sudo k3s kubectl -n golang-be rollout restart deploy/api deploy/worker
sudo k3s kubectl -n golang-be rollout status deploy/api
```

Zero-downtime: API `replicas: 2`, `maxUnavailable: 0`, readiness `/health/ready`, `preStop` drain.

---

## Teardown (bring everything down)

### Full lab wipe (app + golang-be data plane)

```bash
sudo k3s kubectl delete namespace golang-be
docker compose down -v
```

### Verify clean

```bash
sudo k3s kubectl get pods -A | grep golang || echo "no golang pods"
docker ps | grep golang-be || echo "no golang-be compose"
curl -s -o /dev/null -w "%{http_code}\n" http://192.168.0.155/health/ready   # expect fail/000
```

---

## Quick start (local laptop — Compose only)

For local development without k3s: api+worker run in Compose.

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

| Service    | URL                           | Credentials                  |
| ---------- | ----------------------------- | ---------------------------- |
| API        | http://localhost:8080         | header `apikey: dev-api-key` |
| Swagger UI | http://localhost:8080/docs    | —                            |
| Grafana    | http://localhost:3000         | `admin` / `admin`            |
| Prometheus | http://localhost:9090         | —                            |
| Jaeger     | http://localhost:16686        | set `OTEL_ENABLED=true`      |
| Metrics    | http://localhost:8080/metrics | —                            |

Prefer host processes? `make infra-up`, then `make api` + `make worker`.

### Demo accounts (migrations)

| Email                  | Password    | Notes                         |
| ---------------------- | ----------- | ----------------------------- |
| `admin@golang-be.dev`  | `admin123`  | RBAC — can delete any product |
| `seller@golang-be.dev` | `seller123` | Owns demo catalog             |
| `buyer@golang-be.dev`  | `user123`   | Use for wishlist + **orders** |

---

## API overview

All `/api/v1` routes require `apikey`; protected routes also need `Authorization: Bearer <accessToken>`.

```json
{ "success": false, "message": "not found", "errorCode": "RESOURCE_NOT_FOUND" }
```

| Method                | Path                                   | Auth                          | Notes                                           |
| --------------------- | -------------------------------------- | ----------------------------- | ----------------------------------------------- |
| `GET`                 | `/health/live`                         | —                             | Liveness                                        |
| `GET`                 | `/health/ready`                        | —                             | Readiness (critical: PG/Redis; optional: Mongo/Typesense) |
| `GET`                 | `/metrics`                             | —                             | Prometheus (includes `golangbe_outbox_pending`) |
| `POST`                | `/api/v1/authentication/register`      | apikey                        | Create account (password min 8)                 |
| `POST`                | `/api/v1/authentication/login`         | apikey                        | Issue tokens                                    |
| `POST`                | `/api/v1/authentication/refresh-token` | apikey                        | Rotate refresh (jti in Redis)                   |
| `POST`                | `/api/v1/authentication/logout`        | apikey                        | Revoke refresh jti                              |
| `GET`                 | `/api/v1/authentication/me`            | +bearer                       | Current user                                    |
| `GET`                 | `/api/v1/products`                     | +bearer                       | Seller's own catalog (cursor + cache)           |
| `GET`                 | `/api/v1/products/catalog`             | +bearer                       | Buyer marketplace browse                        |
| `GET`                 | `/api/v1/products/search?q=`           | +bearer                       | Typesense full-text                             |
| `POST`                | `/api/v1/products`                     | +bearer                       | Create → Kafka event (async)                    |
| `GET`/`PUT`/`DELETE`  | `/api/v1/products/:id`                 | +bearer                       | Detail / update / delete                        |
| `GET`/`POST`/`DELETE` | `/api/v1/wishlists`                    | +bearer                       | Wishlist (+ Redis count)                        |
| `POST`                | `/api/v1/orders`                       | +bearer + **Idempotency-Key** | Create order (TX outbox + stock hold)           |
| `GET`                 | `/api/v1/orders` / `/:id`              | +bearer                       | List / detail                                   |
| `POST`                | `/api/v1/orders/:id/cancel`            | +bearer                       | Cancel + release stock                          |
| `POST`                | `/api/v1/orders/:id/pay`               | +bearer                       | Lab payment override (`success\|fail\|timeout`) |
| `POST`                | `/api/v1/orders/:id/fulfill`           | +bearer **admin**             | `paid` → `fulfilled`                            |
| `GET`                 | `/api/v1/lab/*`                        | +bearer **admin**, non-prod   | Infra / outbox chaos                            |
| `GET`                 | `/ws/products?token=&apikey=`          | JWT + apikey                  | Real-time events                                |

Full contract: [`docs/openapi.yaml`](docs/openapi.yaml) · Postman collection + env: [`docs/postman/`](docs/postman/) (also served under `/docs` when non-prod if you copy assets — primary import is from repo).

---

## Project layout

```text
cmd/
  api/                      HTTP API process (Gin + fx) — migrate, outbox relay, notifier
  worker/                   Kafka worker (fx) — dispatch → payment / inventory / audit

internal/
  http/                     ★ everything for the API process
    server/                 Gin engine wiring
    middleware/             auth, apikey, ratelimit, ...
    health/                 /health/live|ready
    auth|product|wishlist|order|lab/
    notify/                 WebSocket + Kafka notifier
  worker/                   ★ everything for the worker process
    dispatch/               route events by type
    payment/                order.created → simulate pay → outbox
    inventory/              commit / release reservations (+ inbox)
    audit/                  Mongo audit + DLQ
  domain/                   shared entities + errors + order state machine
  config/                   shared env config
  platform/                 db, redis, kafka, mongo, typesense, telemetry,
                            outbox, inbox, ledger
  metrics/                  shared Prometheus metrics

migrations/                 000001…000007 (commerce + inbox composite PK)
scripts/deploy-k3s-lab.sh   single-node: Compose data plane + k3s app
scripts/load-orders.sh      Legacy bash oversell (prefer make bench-oversell / k6)
k8s/ helm/                  deploy
chaos/                      LitmusChaos engines (pod-delete api/worker)
.gitlab-ci.yml              GitLab CI (self-deploy or pipeline)
docs/                       OpenAPI, belajar/ (Indonesian code walkthrough), PANDUAN (Indonesian ops), GitLab, public access
scripts/litmus-install.sh   Install Litmus operator + experiment (once)
scripts/litmus-run.sh       Run LitmusChaos engines (make chaos)
scripts/chaos-outbox.sh     App outbox dual-write SLI (make chaos-outbox)
scripts/lab-restore.sh      Restore containers / outbox / rate limit after chaos or bench
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

## Load testing (Grafana k6)

Capacity evidence lives in [`docs/BENCHMARK.md`](docs/BENCHMARK.md) — not in feature bullets.
Infra chaos: **LitmusChaos** — [`chaos/README.md`](chaos/README.md) + `make chaos`.
Outbox dual-write SLI: `make chaos-outbox`.

```bash
# Syntax check (Docker; no live API):
make k6-validate

# API must be up. Lab example:
BASE_URL=http://192.168.0.155 API_KEY=lab-api-key-change-in-prod make bench

make bench-smoke      # health + order path
make bench-oversell   # N≫S race, stock never < 0
make bench-checkout   # sustained POST /orders (raise RATE_LIMIT_MAX first)
```

## Chaos testing (LitmusChaos)

```bash
# Once on the k3s lab box:
SUDO_PASS='…' make litmus-install

HOST=http://192.168.0.155 API_KEY=lab-api-key-change-in-prod make chaos
make chaos-api        # pod-delete on api only
make chaos-outbox     # HTTP outbox pause/resume SLI
```

| Layer | Proves |
| ----- | ------ |
| `test/unit` + `test/integration` | Logic & HTTP wiring (mocks / in-memory) |
| `load/*` + `scripts/bench.sh` (k6) | Live SLIs: oversell, checkout latency, rate limit, mixed traffic |
| `chaos/` + Litmus | Pod kill resilience (api/worker) + ready probes |

Results → `load/results/` (gitignored). Paste a row into the table in `docs/BENCHMARK.md` after each honest run.

---

## CI/CD (GitLab)

See [`docs/GITLAB-SETUP.md`](docs/GITLAB-SETUP.md) and [`.gitlab-ci.yml`](.gitlab-ci.yml).

| Stage               | Purpose                                                         |
| ------------------- | --------------------------------------------------------------- |
| lint / test / build | Every branch & MR                                               |
| docker              | Image build + Trivy                                             |
| release             | Push to GitLab Container Registry (`main` / tags)               |
| deploy              | **Manual** job — pull images on the server and rollout (Mode B) |

---

## Kubernetes (k3s)

The cluster runs only the **stateless app tier** (`api` + `worker`). Data services stay on Docker Compose on the same host.

Day-to-day commands: see **[Lab deploy (Mode A)](#lab-deploy-mode-a--commands)** and **[Teardown](#teardown-bring-everything-down)** above.

Public / domain access later: [`docs/PUBLIC-ACCESS.md`](docs/PUBLIC-ACCESS.md).

```bash
# Helm alternative (same split: app in cluster, data on Compose)
helm install golang-be ./helm/golang-be -n golang-be --create-namespace \
  --set config.hostIP=192.168.0.155
```

---

## Design decisions worth reading

- **Migrations over AutoMigrate** — reviewed, ordered, reversible SQL (`000001`…`000007`)
- **Outbox for critical paths** — orders never depend on Kafka being up at request time
- **Product async publish kept on purpose** — contrast with outbox; teach the dual-write failure mode
- **At-least-once + inbox `(event_id, consumer)`** — Kafka redelivery must not double-pay; multi-handler safe
- **Hold TTL** — unpaid reservations auto-expire so stock is not stuck forever
- **Payment ownership** — worker auto-charge is canonical; HTTP pay is a teaching override
- **Stock ledger** — every hold/release/commit is an append-only movement
- **Circuit breaker on search** — Typesense outage does not kill CRUD
- **Rate limit** — fail-closed on auth routes if Redis is down; fail-open elsewhere
- **Async side effects on product** — Kafka/Typesense do not inflate HTTP latency
- **Config validation at boot** — misconfig fails fast, including production secret / CORS guards
- **Lab surface** — admin-only and disabled in production

Learning path (Indonesian): start at [`docs/belajar/README.md`](docs/belajar/README.md).
