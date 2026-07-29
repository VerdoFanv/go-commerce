# Golang BE

Simple RESTful Go backend dengan clean architecture.

## Stack

| Layer | Tech |
|-------|------|
| HTTP | Gin |
| ORM | GORM |
| DB | PostgreSQL |
| Cache | go-redis |
| Queue | RabbitMQ (`amqp091-go`) |
| Auth | JWT + `apikey` header |

## Arsitektur

```
Request → Handler → Service → Repository → PostgreSQL
                      ↓           ↓
                   RabbitMQ     Redis
                   (goroutine)
```

Contoh goroutine di project:
- `cmd/api` — HTTP server di `go func()` supaya main bisa graceful shutdown
- `product` create — publish `product.created` ke RabbitMQ secara async

Gaya coding mengikuti pola feature-folder (mirip Wisteria/Jangkau) + layering dari kurikulum Go kamu:

- `handler.go` — HTTP only
- `service.go` — business rules, cache, publish event
- `repository.go` — GORM
- `domain/` — entity + sentinel errors
- Response envelope seperti Wisteria mobile: `{ success, message, data }` + JSON **camelCase**

## Quick start

```bash
cp .env.example .env

# butuh Docker
make infra-up

make deps
make api      # terminal 1 — HTTP :8080
make worker   # terminal 2 — RabbitMQ consumer
```

API docs (Swagger UI): [http://localhost:8080/docs](http://localhost:8080/docs)  
OpenAPI raw: [http://localhost:8080/docs/openapi.yaml](http://localhost:8080/docs/openapi.yaml)

RabbitMQ management UI: [http://localhost:15672](http://localhost:15672) (`guest` / `guest`)

## Contoh request

Semua request ke `/api/v1` butuh header:

```http
apikey: dev-api-key
```

### Register

```bash
curl -s http://localhost:8080/api/v1/authentication/register \
  -H 'Content-Type: application/json' \
  -H 'apikey: dev-api-key' \
  -d '{"name":"Andi","email":"andi@example.com","password":"secret1"}'
```

### Login

```bash
curl -s http://localhost:8080/api/v1/authentication/login \
  -H 'Content-Type: application/json' \
  -H 'apikey: dev-api-key' \
  -d '{"email":"andi@example.com","password":"secret1"}'
```

### Create product (publish event `product.created`)

```bash
curl -s http://localhost:8080/api/v1/products \
  -H 'Content-Type: application/json' \
  -H 'apikey: dev-api-key' \
  -H "Authorization: Bearer <accessToken>" \
  -d '{"name":"Kopi Susu","description":"Iced","price":28000,"stock":10}'
```

Worker akan log event yang diterima dari queue.

## API surface

| Method | Path | Auth | Keterangan |
|--------|------|------|------------|
| GET | `/api/v1/health` | apikey | Health check |
| POST | `/api/v1/authentication/register` | apikey | Register |
| POST | `/api/v1/authentication/login` | apikey | Login |
| POST | `/api/v1/authentication/refresh-token` | apikey | Refresh JWT |
| GET | `/api/v1/authentication/me` | apikey + bearer | Profile |
| GET | `/api/v1/products` | apikey + bearer | List (Redis cache) |
| POST | `/api/v1/products` | apikey + bearer | Create + MQ event |
| GET | `/api/v1/products/:id` | apikey + bearer | Detail (Redis cache) |
| PUT | `/api/v1/products/:id` | apikey + bearer | Update |
| DELETE | `/api/v1/products/:id` | apikey + bearer | Delete |

Detail schema lengkap ada di `docs/openapi.yaml`.

## Struktur

```
cmd/
  api/          # HTTP server
  worker/       # RabbitMQ consumer
internal/
  auth/
  product/
  health/
  config/
  domain/
  middleware/
  platform/     # postgres, redis, rabbitmq
pkg/response/
docs/
.cursor/rules/
```

## Cursor rules

Ada di `.cursor/rules/`:

- `golang-be-overview.mdc` — arsitektur & kontrak
- `golang-code-style.mdc` — gaya kode Go
- `golang-api-conventions.mdc` — REST, auth, Redis, RabbitMQ
