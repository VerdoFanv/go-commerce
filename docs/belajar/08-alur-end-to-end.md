# 08 — Alur end-to-end

Tujuan: satukan semua modul dalam **satu cerita request**. Baca sambil buka file yang disebut.

---

## Cerita A — Create product (happy path)

```text
Client
  │  POST /api/v1/products
  │  headers: apikey, Authorization Bearer
  ▼
Gin middleware chain
  RequestID → Tracing → metrics → security → CORS → timeout → logger
  ▼
APIKey + RateLimit (/api/v1)
  ▼
product.Handler.create
  BindJSON → UserID dari JWT
  ▼
product.Service.Create
  │  1. repo.Create (Postgres)
  │  2. invalidate list cache (Redis)
  │  3. go publishAsync → Kafka products.events (product.created)
  │  4. go indexAsync → Typesense
  ▼
response.Created (JSON envelope)
```

**Paralel setelah publish:**

```text
Kafka topic products.events
        │
        ├─ consumer group WORKER (cmd/worker)
        │     Fetch → audit.Processor.Handle → Mongo Insert → Commit
        │
        └─ consumer group NOTIFIER (cmd/api)
              Fetch → Hub.Broadcast → semua WebSocket /ws/products → Commit
```

**Yang harus kamu lihat di lab:**

1. Row baru di Postgres (`/lab/postgres/samples` atau SQL).
2. Event di Mongo (`/lab/mongo/events`).
3. Dokumen di Typesense (`/lab/typesense?q=...`).
4. (Opsional) pesan di `websocat` WS.
5. Metric `events_published` / `consumed` naik.

---

## Cerita B — Get product (cache)

```text
GET /products/:id
  → Service.GetByID
      → Redis GET product:{id}
           hit  → return
           miss → Postgres → SET Redis TTL → return
```

Eksperimen: `GET` → `/lab/redis/product/:id` (lihat hit) → `DELETE` invalidate → `GET` lagi (miss lalu set ulang).

---

## Cerita C — Wishlist count

```text
GET /wishlists/count
  → Redis wishlist:count:{userId}
       miss → COUNT Postgres → SET Redis
POST/DELETE wishlist → invalidate count key
```

---

## Cerita D — Login → me

```text
POST /authentication/login
  → bcrypt compare → issueTokens (access + refresh)
GET /authentication/me + Bearer access
  → Auth middleware Claims → Service.Me → User tanpa password
```

---

## Cerita E — Poison / DLQ (worker)

```text
Message tidak bisa di-decode (ID kosong)
  → Handle → deadLetter → topic products.events.dlq
  → offset tetap di-commit (agar tidak stuck forever)
```

Atau Mongo gagal 3x → DLQ dengan reason insert failed.

---

## Peta “mau ubah X, edit Y”

| Mau ubah… | Edit… |
|-----------|--------|
| Format JSON sukses/gagal | `pkg/response` |
| Field JWT claims | `middleware/auth.go` + `auth/service.issueTokens` |
| TTL cache product | `PRODUCT_CACHE_TTL` + `product/service` |
| Nama topic / group | `config` + env |
| Schema users/products | `migrations/*.sql` (bukan AutoMigrate ad-hoc) |
| Retry worker | `maxRetries` / backoff di `audit.go` |
| Route baru | `handler.RegisterRoutes` + pastikan di-wire `server.NewEngine` |
| Dependency baru di boot | `cmd/api/main.go` atau `cmd/worker/main.go` (`fx.Provide`) |

---

## Urutan baca ulang (setelah E2E)

1. [01-peta-folder](01-peta-folder.md)  
2. [02-entrypoints](02-entrypoints.md)  
3. Feature yang paling sering kamu sentuh di [06](06-http-features.md)  
4. Ops praktek: [`../PANDUAN-BELAJAR.md`](../PANDUAN-BELAJAR.md)

Kalau ada file Go baru yang belum masuk dokumen ini, update file belajar yang relevan — jangan numpuk lagi ke satu README raksasa.
