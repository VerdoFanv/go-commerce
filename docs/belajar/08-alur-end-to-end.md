# 08 — Alur end-to-end

Tujuan: satukan modul dalam **cerita request**. Buka file yang disebut sambil baca.

---

## Cerita A — Create product (fire-and-forget)

```text
POST /api/v1/products
  → middleware → product.Handler → Service.Create
      1. Postgres insert
      2. Redis cache
      3. go publishAsync → Kafka (bisa hilang jika broker down)
      4. go indexAsync → Typesense
  → 201

Kafka products.events
  ├─ worker group → dispatch → (bukan order) → audit Mongo
  └─ notifier group → WebSocket /ws/products
```

Pakai ini untuk belajar pipeline lama + WS. **Jangan** jadikan pola publish-nya model untuk payment/order.

---

## Cerita B — Create order (outbox + worker) ★

```text
Client
  POST /api/v1/orders
  headers: apikey, Bearer, Idempotency-Key
  ▼
order.Service.Create
  ▼
Postgres BEGIN
  hold stock (UPDATE … WHERE stock >= qty)
  insert orders + items + reservations(held)
  stock_ledger hold
  outbox_events order.created
COMMIT → 201 pending_payment
  ▼
(idempotency_keys menyimpan body 201 untuk replay)

API outbox.Relay
  SELECT unpublished → Kafka Publish → published_at

Worker dispatch
  payment: inbox → charge → order.paid outbox (+ ledger commit)
  (relay publish order.paid)
  inventory: inbox → reservation committed
  audit: Mongo event_audit
```

**Chaos belajar:**

```bash
POST /lab/outbox/pause
POST /orders          # tetap 201
GET  /lab/outbox/pending
POST /lab/outbox/resume
POST /lab/outbox/relay-once
# poll GET /orders/:id sampai paid
POST /orders/:id/fulfill
```

---

## Cerita C — Get product (cache bypass)

```text
GET /products/:id
  Redis hit → return
  Redis error/miss → Postgres → SET cache (error SET diabaikan)
```

Redis down ≠ request gagal **di dalam proses**. Tapi Redis = **critical readiness** → k3s mengeluarkan pod dari Service sampai Redis kembali. Bedakan “handler FO” vs “cluster traffic”.

Setelah order hold/release, cache `product:{id}` di-invalidate (jangan percaya stock stale).

---

## Cerita D — Wishlist count

```text
GET /wishlists/count → Redis atau COUNT Postgres
POST/DELETE wishlist → invalidate count key
```

---

## Cerita E — Poison / DLQ

```text
Event tak ter-decode (ID kosong)
  → audit deadLetter → products.events.dlq
  → offset commit (jangan stuck forever)
```

---

## Cerita F — Infra mati (lanjut ke modul 10)

Ringkas:

| Dep | Ready | Commerce HTTP |
|-----|-------|---------------|
| Typesense down | degraded | CRUD OK; search 503 |
| Mongo down | degraded | API OK; audit worker telat/DLQ |
| Kafka down | (bukan ready check) | Order 201 + outbox; product event bisa hilang |
| Outbox pause | — | Order 201; pending naik sampai resume |

Full matrix + script: [10-failure-ops.md](10-failure-ops.md) · [`../FAILURE-RUNBOOK.md`](../FAILURE-RUNBOOK.md)

---

## Peta “mau ubah X, edit Y”

| Mau ubah… | Edit… |
|-----------|--------|
| Format JSON | `pkg/response` |
| JWT claims | `middleware/auth.go` + `auth/service.issueTokens` |
| TTL cache product | `PRODUCT_CACHE_TTL` + `product/service` |
| Outbox poll interval | `platform/outbox` Relay |
| State machine order | `domain/order.go` |
| Payment outcome default | `worker/payment` |
| Schema | `migrations/*.sql` (bukan AutoMigrate) |
| Route baru | `handler.RegisterRoutes` + `server.NewEngine` |
| Boot wiring | `cmd/api` / `cmd/worker` |
| Ready critical vs optional | `internal/http/health/` |

---

## Urutan baca ulang

1. [01-peta-folder](01-peta-folder.md)  
2. [09-commerce-reliability](09-commerce-reliability.md) kalau fokus bisnis  
3. [10-failure-ops](10-failure-ops.md) untuk chaos nyata  
4. Ops: [`../PANDUAN-BELAJAR.md`](../PANDUAN-BELAJAR.md) · Postman: [`../postman/`](../postman/)
