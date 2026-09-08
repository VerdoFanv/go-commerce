# 05 — HTTP server & middleware

Tujuan: paham **urutan middleware** dan permukaan non-feature (`/health`, `/metrics`, `/docs`, pprof).

Buka: `internal/http/server/server.go` + `internal/http/middleware/*`.

---

## `server.NewEngine` — susunan Gin

Urutan `Use` (atas → bawah = luar → dalam):

| # | Middleware | Fungsi |
|---|------------|--------|
| 1 | `gin.Recovery` | Tangkap panic → 500, proses tetap hidup |
| 2 | `RequestID` | `X-Request-Id` (generate atau teruskan) |
| 3 | `Tracing` | Span OTel per request |
| 4 | `metrics.Middleware` | RED Prometheus |
| 5 | `SecurityHeaders` | Header keamanan dasar |
| 6 | CORS | Origin `*`, header `apikey` / `Authorization` |
| 7 | `Timeout` | Batas waktu request (`REQUEST_TIMEOUT`) |
| 8 | `RequestLogger` | Log method/path/status/latency |

Lalu route:

| Path | Auth | Fungsi |
|------|------|--------|
| `/health/live`, `/health/ready` | tidak | Probe k8s |
| `/metrics` | tidak | Prometheus scrape |
| `/debug/pprof/*` | tidak (non-prod) | Profiling |
| `/api/v1/*` | **apikey** + rate limit | Semua REST fitur |
| `/ws/products` | JWT query/header | WebSocket notify |
| `/docs` | tidak | Static Swagger UI |

`NewHTTPServer` — bungkus engine di `net/http.Server` supaya `Shutdown` graceful.

---

## Middleware file-by-file

### `apikey.go` — `APIKey(expected)`

- Baca header `apikey` (atau alias yang diizinkan CORS).
- Salah → `domain.ErrInvalidAPIKey` / 401.
- Dipasang di group `/api/v1` saja.

### `auth.go` — JWT

| Symbol | Fungsi |
|--------|--------|
| `Claims` | `userId`, `role`, expiry |
| `Auth(secret)` | Middleware: Bearer wajib, set context |
| `ParseToken` | Dipakai refresh / WS authenticate |
| `UserID` / `UserRole` | Helper dari `gin.Context` |

### `rbac.go` — `RequireRole(roles...)`

Cek role dari context setelah `Auth`. (Siap dipakai route admin; product delete juga cek role di service.)

### `ratelimit.go`

| Symbol | Fungsi |
|--------|--------|
| `RateLimiter` | Interface Incr/window |
| `NewRedisLimiter` | Implementasi Redis |
| `RateLimit(limiter, max, window)` | 429 + `ErrRateLimited` |

Tanpa Redis, limiter nil → rate limit no-op (lihat wiring server).

### `timeout.go`

Cancel context kalau request lebih lama dari config.

### `requestid.go` / `logger.go` / `security.go` / `tracing.go`

Observability + hygiene — tidak ubah business logic.

---

## `health/`

| Symbol | Fungsi |
|--------|--------|
| `Handler.live` | Proses hidup (selalu OK) |
| `Handler.ready` | Ping semua `Checker` (Postgres, Redis, Mongo, Typesense) |
| `PostgresCheck` / `RedisCheck` / `MongoCheck` / `TypesenseCheck` | Adapter ping |

k3s readiness pakai `/health/ready` — pod tidak terima traffic kalau dependency kritis down.

---

## Latihan

1. Gambar urutan middleware di kertas, cocokkan dengan `NewEngine`.
2. Hit `/api/v1/...` tanpa `apikey` — pastikan gagal sebelum JWT.
3. Bandingkan `/health/live` vs `/health/ready` saat Redis dimatikan.

Lanjut → [06-http-features.md](06-http-features.md)
