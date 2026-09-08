# 03 — Shared core (`domain`, `config`, `metrics`, `pkg`, migrations)

Tujuan: paham fondasi yang dipakai API **dan** worker — tanpa ikut detail HTTP/Kafka.

---

## `internal/domain/` — kontrak bisnis bersama

Tidak ada dependency ke Gin/GORM/Kafka. Layer luar yang bergantung ke sini, bukan sebaliknya.

### Entity

| File | Tipe | Fungsi |
|------|------|--------|
| `user.go` | `User`, `AuthTokens`, `AuthResult` | Bentuk user + hasil login/register yang aman dikembalikan ke API |
| `user.go` | const `RoleUser`, `RoleAdmin` | Role string di JWT/SQL; seed demo juga bisa pakai role lain di data |
| `product.go` | `Product` | Entity produk (bukan GORM model) |
| `product.go` | `EventProductCreated/Updated/Deleted` | Nama event Kafka seragam |

### Sentinel errors (`errors.go`)

| Error | Kapan dipakai | `ErrorCode` |
|-------|---------------|-------------|
| `ErrNotFound` | resource hilang | `RESOURCE_NOT_FOUND` |
| `ErrInvalid` | validasi / input jelek | `VALIDATION_ERROR` |
| `ErrUnauthorized` | belum login / kredensial salah | `AUTH_UNAUTHORIZED` |
| `ErrForbidden` | role tidak cukup | `AUTH_FORBIDDEN` |
| `ErrEmailTaken` | register email duplikat | `AUTH_EMAIL_TAKEN` |
| `ErrConflict` | conflict umum (mis. wishlist duplikat) | `RESOURCE_CONFLICT` |
| `ErrRateLimited` | terlalu banyak request | `RATE_LIMIT_EXCEEDED` |
| `ErrUnavailable` | dependency down | `SERVICE_UNAVAILABLE` |
| `ErrTokenExpired` | JWT expired (string sengaja kontrak Wisteria) | `AUTH_TOKEN_EXPIRED` |
| `ErrInvalidAPIKey` | header `apikey` salah | `AUTH_INVALID_API_KEY` |

`ErrorCode(err)` — handler map ke field `errorCode` di JSON, biar client tidak parse pesan bebas.

**Pola belajar:** di service `return domain.ErrNotFound`, di handler `errors.Is` + `response.FailCode`.

---

## `internal/config/` — satu sumber env

| Symbol | Fungsi |
|--------|--------|
| `Config` | Semua knob: DB, Redis, Kafka, Mongo, Typesense, JWT, rate limit, timeout, metrics port |
| `Load()` | Baca env + default → `Validate()` → panik kalau invalid (fail-fast boot) |
| `Validate()` | Struct tags `go-playground/validator` |
| `PostgresDSN()` | String koneksi GORM/pg |
| `IsProduction()` | `APP_ENV == production` |
| `env` / `envInt` / `envBool` / `envDuration` / `envList` | Helper parse env |

Produksi: `Load` menolak `API_KEY` / `JWT_SECRET` default.

**Jangan** hardcode credential di kode feature — selalu lewat `config.Config`.

---

## `internal/metrics/` — Prometheus

| Metric | Arti |
|--------|------|
| `golangbe_http_requests_total` | RED: jumlah request (method, route, status) |
| `golangbe_http_request_duration_seconds` | Latency histogram |
| `golangbe_events_published_total` | Event ke Kafka (by type) |
| `golangbe_events_consumed_total` | Event dikonsumsi (by type + consumer) |
| `golangbe_events_dead_lettered_total` | Ke DLQ |
| `golangbe_outbox_pending` | Gauge: baris outbox belum publish (lag relay) |
| `golangbe_websocket_active_connections` | Gauge koneksi WS |

`Middleware()` — dipasang di Gin engine; catat setelah `c.Next()`.

API expose `/metrics` di `AppPort`. Worker expose di `MetricsPort` (default `2112`).

---

## `pkg/response/` — envelope JSON

Kontrak Wisteria-style:

```json
{ "success": true, "message": "success", "data": {} }
{ "success": false, "message": "...", "errorCode": "..." }
```

| Fungsi | Kapan |
|--------|-------|
| `OK` / `Created` / `Success` | Sukses 200/201 |
| `OKWithMeta` / `SuccessWithMeta` | List + pagination meta |
| `Fail` / `FailCode` / `FailWithErrors` | Error + optional code / field errors |
| `BindJSON` | Bind + validate body; return error siap di-map handler |

Semua handler HTTP harus lewat package ini — jangan tulis JSON ad-hoc.

---

## `migrations/` — schema Postgres versioned

| File | Isi |
|------|-----|
| `000001_init` | Tabel inti (users, products, …) |
| `000002_seed_admin` | Admin awal |
| `000003_seed_demo_data` | Seller/buyer + produk demo |
| `000004_wishlists` | Tabel wishlist + seed |
| `000005_commerce` | orders, items, reservations, payments, idempotency_keys, outbox_events |
| `000006_inbox_ledger` | `processed_events` (inbox) + `stock_ledger` |
| `migrations.go` | Embed SQL + runner (`database.Migrate`) |

Hanya **API** yang migrate saat OnStart. Worker memakai schema yang sama, tidak mengubahnya.

Domain order: lihat `internal/domain/order.go` (status, event types, `CanTransition`).

---

## Checklist latihan

1. Tambah sentinel error baru → isi juga cabang di `ErrorCode`.
2. Cari satu handler yang `errors.Is(err, domain.ErrX)` — tiru polanya.
3. Baca `Config` field Kafka — cocokkan dengan env di `.env.example`.

Lanjut → [04-platform.md](04-platform.md)
