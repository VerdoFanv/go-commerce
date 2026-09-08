# 02 — Entrypoints (`cmd/api` & `cmd/worker`)

Tujuan: paham **proses hidup** aplikasi — siapa yang di-construct, kapan migrate, kapan listen, kapan shutdown.

Kedua binary pakai **Uber fx**: kamu daftar constructor (`fx.Provide`), fx resolve dependency graph, lalu `fx.Invoke` lifecycle.

---

## `cmd/api/main.go` — proses API

### Yang dilakukan `main()`

1. `godotenv.Load()` — baca `.env` (lokal); di k8s env sudah di-inject.
2. `fx.New(...).Run()` — bangun graph + block sampai SIGINT/SIGTERM.

### Graph yang di-Provide (ringkas)

| Grup | Constructor | Hasil |
|------|-------------|--------|
| Config | `config.Load` | `Config` |
| Platform | `database.Connect`, `redis.Connect`, `mongo.Connect`, `typesense.Connect`, `telemetry.Setup` | client infra |
| Kafka | `NewProducer(products)`, `NewConsumer(notifier group)` | publish + consume di proses API |
| Adapter iface | Producer → `product.EventPublisher` / `lab.EventPublisher`; Typesense → `SearchEngine` | service tidak import kafka/typesense konkret |
| Domain HTTP | `auth|product|wishlist|lab` NewRepository → NewService → NewHandler | fitur |
| Ops | `health.NewHandler`, `notify` Hub/Notifier/Handler | probe + WS |
| Server | `server.NewEngine`, `server.NewHTTPServer` | Gin + `net/http.Server` |

### Lifecycle `registerLifecycle`

**OnStart (urutannya penting):**

1. `database.Migrate(db)` — jalankan SQL migrations (hanya API yang migrate).
2. `kafka.EnsureTopics(...)` — pastikan topic ada (dev convenience).
3. `go notifier.Run(...)` — consumer group **notifier** → WebSocket.
4. `go httpServer.ListenAndServe()` — terima traffic.

**OnStop:**

1. Cancel notifier.
2. `httpServer.Shutdown` — graceful (zero-downtime drain).
3. Close Kafka consumer/producer, Mongo, Redis, SQL.
4. `telemetry.Shutdown`.

### Kenapa notifier ada di API, bukan worker?

- Worker fokus **audit durable** (Mongo + DLQ).
- Notifier fokus **push real-time** ke client yang sedang connect ke API.
- Satu topic, **dua consumer group** berbeda = dua tanggung jawab.

---

## `cmd/worker/main.go` — proses worker

### Graph

| Provide | Fungsi |
|---------|--------|
| `config.Load` | env |
| `mongo.Connect` → `audit.NewMongoStore` → `audit.Store` | sink audit |
| `kafka.NewConsumer(products, group=worker)` | baca event |
| `kafka.NewProducer(DLQ topic)` | kirim pesan gagal |
| `audit.NewProcessor` | business worker |

### Lifecycle

**OnStart:**

1. `EnsureTopics`.
2. `store.EnsureIndexes` — unique `eventId` (idempotent audit).
3. Listen `:METRICS_PORT` untuk Prometheus (`/metrics`).
4. Loop: `Fetch` → `processor.Handle` → `Commit` (hanya jika Handle sukses).

**OnStop:** stop loop, shutdown metrics server, close consumer/DLQ/mongo.

### At-least-once

Offset **tidak** di-commit sebelum side effect (Mongo insert) sukses.  
Kalau crash setelah insert tapi sebelum commit → message bisa diproses lagi → unique `eventId` mencegah duplikat dokumen.

---

## Latihan

1. Di `cmd/api/main.go`, hitung berapa `fx.Provide` — cocokkan dengan tabel di atas.
2. Matikan sementara `EnsureTopics` di log — apa yang terjadi kalau topic belum ada?
3. Bandingkan consumer group string: API pakai `KafkaGroupNotifier`, worker pakai `KafkaGroupWorker`.

Lanjut → [03-shared-core.md](03-shared-core.md)
