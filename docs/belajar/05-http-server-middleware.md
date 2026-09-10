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
| 5 | `SecurityHeaders(enableHSTS)` | OWASP headers (+ HSTS jika `ENABLE_HSTS=true`) |
| 6 | CORS | `CORS_ORIGINS` (prod **melarang** `*`); header `apikey` / `Authorization` / **`Idempotency-Key`** |
| 7 | `Timeout` | Batas waktu request (`REQUEST_TIMEOUT`) |
| 8 | `RequestLogger` | Log method/path/status/latency (tanpa body) |

Lalu route:

| Path | Auth | Fungsi |
|------|------|--------|
| `/health/live`, `/health/ready` | tidak | Probe k8s |
| `/metrics` | tidak | Prometheus scrape (isolasi di edge kalau public) |
| `/debug/pprof/*` | non-prod only | Profiling |
| `/api/v1/*` | **apikey** + rate limit | Semua REST fitur |
| `/ws/products` | `apikey` + JWT access (`?token=` / `?apikey=`) | WebSocket notify |
| `/docs` | non-prod only | Static Swagger UI |

`NewHTTPServer` — bungkus engine di `net/http.Server` supaya `Shutdown` graceful.

---

## Middleware file-by-file

### `apikey.go` — `APIKey(expected)`

- Baca header `apikey` atau `X-API-Key`.
- Bandingkan dengan **constant-time** (`subtle.ConstantTimeCompare`).
- Salah → `domain.ErrInvalidAPIKey` / 401.
- Dipasang di group `/api/v1` saja.

### `auth.go` — JWT

| Symbol | Fungsi |
|--------|--------|
| `Claims` | `userId`, `role`, `type`, expiry (+ `jti` di refresh) |
| `Auth(secret)` | Middleware: Bearer wajib, HMAC + `type=access`, set context |
| `ParseAccessToken` / `ParseToken` | Sama ketatnya — dipakai WS |
| `UserID` / `UserRole` | Helper dari `gin.Context` |

### `rbac.go` — `RequireRole(roles...)`

Dipakai nyata di: **lab** (admin), **order fulfill** (admin). Product delete juga cek role di service.

### `ratelimit.go`

| Symbol | Fungsi |
|--------|--------|
| `RateLimiter` | Interface sliding window |
| `NewRedisLimiter` | Implementasi Redis |
| `RateLimit(limiter, max, window)` | 429 + `ErrRateLimited` |

Perilaku Redis error: **fail-closed** pada path `/authentication/*`, fail-open di route lain (tetap layani traffic).

### `timeout.go` / `requestid.go` / `logger.go` / `security.go` / `tracing.go`

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
4. Di prod: pastikan lab route 404 dan `/docs` tidak terdaftar.

Lanjut → [06-http-features.md](06-http-features.md)
