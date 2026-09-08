# 04 — Platform adapters (`internal/platform/`)

Tujuan: paham **cara app bicara ke infra**. Feature service tidak import driver mentah; mereka pakai wrapper di sini (atau interface kecil di package feature).

---

## `platform/database` — PostgreSQL + GORM

| Symbol | Fungsi |
|--------|--------|
| `Connect(cfg)` | Buka `*gorm.DB` dari DSN config |
| `Migrate(db)` | Jalankan SQL migrations embed |

Source of truth relasional: users, products, wishlists.

---

## `platform/redis`

| Symbol | Fungsi |
|--------|--------|
| `Connect` | Client go-redis |
| `Get` / `Set` / `Del` | Cache string/JSON |
| `Raw()` | Akses `*redis.Client` (rate limiter) |
| `Close` | Shutdown |

**Pemakaian di app:**

- Product: `product:{id}`, first-page list `products:user:{id}`
- Wishlist: `wishlist:count:{userId}`
- Middleware: key rate limit `rl:*`

---

## `platform/mongo`

| Symbol | Fungsi |
|--------|--------|
| `Connect` | Client Mongo dari `MONGO_URI` |
| `Collection(name)` | Collection di DB audit |
| `Ping` / `Close` | Health + shutdown |

Worker + lab baca/tulis audit lewat `worker/audit` store, bukan langsung dari banyak tempat.

---

## `platform/kafka`

| Symbol | Fungsi |
|--------|--------|
| `Event` / `NewEvent` | Envelope event (id, type, payload, createdAt) |
| `Producer` / `NewProducer` / `Publish` / `Close` | Publish ke topic |
| `Consumer` / `NewConsumer` / `Fetch` / `Commit` / `Close` | Consume group |
| `Message` / `RawBody` | Hasil fetch + body mentah (untuk DLQ) |
| `EnsureTopics` | Buat topic jika belum ada (dev/lab) |

**Dua consumer group, satu topic products:**

| Group (config) | Proses | Tujuan |
|----------------|--------|--------|
| `KafkaGroupWorker` | `cmd/worker` | Persist audit Mongo + DLQ |
| `KafkaGroupNotifier` | `cmd/api` (notify) | Broadcast WebSocket |

Commit offset hanya setelah side effect sukses (at-least-once).

---

## `platform/typesense`

| Symbol | Fungsi |
|--------|--------|
| `Connect` | HTTP client Typesense |
| `EnsureProductsCollection` | Schema collection products |
| `IndexProduct` / `DeleteProduct` | Sync dokumen |
| `Search` | Full-text search |
| `Ping` / `Stats` | Health + lab explore |

Product service panggil index/delete **async** (goroutine) supaya latency API tidak menunggu search engine.

Kalau Typesense kosong/mati, search bisa degrade — lab punya `reindex` dari Postgres.

---

## `platform/telemetry` — OpenTelemetry

| Symbol | Fungsi |
|--------|--------|
| `Setup(ctx, cfg, serviceName)` | Tracer provider OTLP (kalau `OTEL_ENABLED`) |
| `Shutdown` | Flush spans |

Dipasang di boot API/worker; middleware `Tracing` inject span per request.

---

## Aturan arsitektur

```text
handler → service → (repository | platform client | interface)
                         ↓
                   GORM / Redis / Kafka / Typesense / Mongo
```

Service **boleh** depend ke interface lokal (`EventPublisher`, `SearchEngine`) yang di-wire di `cmd/api` ke implementasi platform — supaya test bisa fake.

Lanjut → [05-http-server-middleware.md](05-http-server-middleware.md)
