# Golang BE

**Production-oriented REST API** built with Go — clean architecture, JWT auth, Redis caching, and async events over RabbitMQ.

[![Go](https://img.shields.io/badge/Go-1.23-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Gin](https://img.shields.io/badge/HTTP-Gin-008ECF)](https://gin-gonic.com/)
[![PostgreSQL](https://img.shields.io/badge/DB-PostgreSQL-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Cache-Redis-DC382D?logo=redis&logoColor=white)](https://redis.io/)
[![RabbitMQ](https://img.shields.io/badge/Queue-RabbitMQ-FF6600?logo=rabbitmq&logoColor=white)](https://www.rabbitmq.com/)
[![CI](https://img.shields.io/badge/CI-GitHub%20Actions-2088FF?logo=githubactions&logoColor=white)](.github/workflows/ci.yml)

---

## Highlights

- **Clean architecture** — `Handler → Service → Repository` with feature packages under `internal/`
- **Auth** — JWT access/refresh tokens + required `apikey` header
- **Caching** — Redis read-through for product list & detail
- **Async events** — `product.created` published to RabbitMQ without blocking HTTP
- **API contract** — envelope `{ success, message, data }` with camelCase JSON
- **Ops-ready** — Docker Compose, multi-stage images, CI lint/test/build, GHCR release

---

## Architecture

```text
                    ┌─────────────────────────────────────┐
                    │              cmd/api                │
                    │         (HTTP :8080 + JWT)          │
                    └─────────────────┬───────────────────┘
                                      │
              Handler ──► Service ──► Repository ──► PostgreSQL
                            │    │
                     Redis ◄─┘    └──► RabbitMQ ──► cmd/worker
```

| Layer          | Responsibility                                        |
| -------------- | ----------------------------------------------------- |
| **Handler**    | Parse/validate HTTP, map domain errors → status codes |
| **Service**    | Business rules, cache, publish events                 |
| **Repository** | GORM / data access only                               |
| **Domain**     | Entities + sentinel errors                            |

Graceful shutdown on the API process; product create publishes MQ events in a background goroutine so responses stay fast.

---

## Tech stack

| Concern   | Choice                        |
| --------- | ----------------------------- |
| Language  | Go **1.23**                   |
| HTTP      | Gin                           |
| ORM       | GORM                          |
| Database  | PostgreSQL 16                 |
| Cache     | go-redis                      |
| Messaging | RabbitMQ (`amqp091-go`)       |
| Auth      | JWT (`golang-jwt`) + `apikey` |
| Lint      | `go vet` + golangci-lint      |
| CI/CD     | GitHub Actions → GHCR         |

---

## Quick start

### 1. Configure

```bash
cp .env.example .env
```

### 2. Run (pick one)

**Full stack (recommended)**

```bash
make docker-up
```

Starts PostgreSQL, Redis, RabbitMQ, API, and worker.

**App locally + infra in Docker**

```bash
make infra-up
make deps
make api      # terminal 1 → :8080
make worker   # terminal 2 → consumer
```

### 3. Open docs

| Service     | URL                                                                                |
| ----------- | ---------------------------------------------------------------------------------- |
| Swagger UI  | [http://localhost:8080/docs](http://localhost:8080/docs)                           |
| OpenAPI     | [http://localhost:8080/docs/openapi.yaml](http://localhost:8080/docs/openapi.yaml) |
| RabbitMQ UI | [http://localhost:15672](http://localhost:15672) (`guest` / `guest`)               |

Stop: `make docker-down`

---

## API overview

All `/api/v1` routes require:

```http
apikey: dev-api-key
```

Protected routes also need:

```http
Authorization: Bearer <accessToken>
```

| Method   | Path                                   | Auth            | Notes              |
| -------- | -------------------------------------- | --------------- | ------------------ |
| `GET`    | `/api/v1/health`                       | apikey          | Liveness / DB ping |
| `POST`   | `/api/v1/authentication/register`      | apikey          | Create account     |
| `POST`   | `/api/v1/authentication/login`         | apikey          | Issue tokens       |
| `POST`   | `/api/v1/authentication/refresh-token` | apikey          | Rotate tokens      |
| `GET`    | `/api/v1/authentication/me`            | apikey + bearer | Current user       |
| `GET`    | `/api/v1/products`                     | apikey + bearer | List (cached)      |
| `POST`   | `/api/v1/products`                     | apikey + bearer | Create + MQ event  |
| `GET`    | `/api/v1/products/:id`                 | apikey + bearer | Detail (cached)    |
| `PUT`    | `/api/v1/products/:id`                 | apikey + bearer | Update (owner)     |
| `DELETE` | `/api/v1/products/:id`                 | apikey + bearer | Delete (owner)     |

Full schemas: [`docs/openapi.yaml`](docs/openapi.yaml)

### Try it

**Register**

```bash
curl -s http://localhost:8080/api/v1/authentication/register \
  -H 'Content-Type: application/json' \
  -H 'apikey: dev-api-key' \
  -d '{"name":"Andi","email":"andi@example.com","password":"secret1"}'
```

**Login**

```bash
curl -s http://localhost:8080/api/v1/authentication/login \
  -H 'Content-Type: application/json' \
  -H 'apikey: dev-api-key' \
  -d '{"email":"andi@example.com","password":"secret1"}'
```

**Create product** (publishes `product.created`)

```bash
curl -s http://localhost:8080/api/v1/products \
  -H 'Content-Type: application/json' \
  -H 'apikey: dev-api-key' \
  -H "Authorization: Bearer <accessToken>" \
  -d '{"name":"Kopi Susu","description":"Iced","price":28000,"stock":10}'
```

**Success envelope**

```json
{
  "success": true,
  "message": "product created",
  "data": { "id": 1, "name": "Kopi Susu", "userId": 1 }
}
```

---

## Project layout

```text
cmd/
  api/                 HTTP server
  worker/              RabbitMQ consumer
internal/
  auth/                Register, login, refresh, me
  product/             CRUD + cache + events
  health/              Health check
  config/              Env-based config
  domain/              Entities & sentinel errors
  middleware/          API key, JWT, logging
  platform/            Postgres, Redis, RabbitMQ adapters
pkg/response/          JSON envelope helpers
test/
  testutil/            Shared test helpers
  mocks/               In-memory repositories
  unit/                Unit tests by feature
  integration/         HTTP API tests
docs/                  OpenAPI + Swagger UI
.github/               CI, release, Dependabot
```

---

## Testing & quality

Centralized `test/` layout (Jest-style). Details: [`test/README.md`](test/README.md)

```bash
make test               # unit + integration
make test-unit
make test-integration
make test-race
make test-cover         # → coverage.html
make lint               # golangci-lint
make ci                 # vet + lint + test + build
```

---

## CI/CD

| Workflow                                 | When                        | What                                                            |
| ---------------------------------------- | --------------------------- | --------------------------------------------------------------- |
| [CI](.github/workflows/ci.yml)           | PR & push to `main`         | vet, lint, tests (+ race), coverage, binary build, Docker build |
| [Release](.github/workflows/release.yml) | `main`, tag `v*`, or manual | Push `api` & `worker` images to **GHCR**                        |
| [Dependabot](.github/dependabot.yml)     | Weekly                      | Go modules, Actions, Docker bases                               |

**Images**

```text
ghcr.io/<owner>/golang-be-api:latest
ghcr.io/<owner>/golang-be-api:sha-<commit>
ghcr.io/<owner>/golang-be-api:1.0.0

ghcr.io/<owner>/golang-be-worker:...
```

**Tag a release**

```bash
git tag v1.0.0
git push origin v1.0.0
```

After the first push, set package visibility under GitHub → Packages as needed.

---

## Make targets

| Target                                | Description                      |
| ------------------------------------- | -------------------------------- |
| `make docker-up`                      | Full stack via Compose           |
| `make docker-down`                    | Tear down Compose                |
| `make infra-up`                       | Postgres + Redis + RabbitMQ only |
| `make api` / `make worker`            | Run processes locally            |
| `make test` / `make lint` / `make ci` | Quality gates                    |

---

## Conventions

Cursor rules live in [`.cursor/rules/`](.cursor/rules/):

| Rule                     | Focus                          |
| ------------------------ | ------------------------------ |
| `golang-be-overview`     | Architecture & API contract    |
| `golang-code-style`      | Go naming & layering           |
| `golang-api-conventions` | Auth, Redis, RabbitMQ patterns |
| `golang-testing`         | `test/` layout & mocks         |

---
