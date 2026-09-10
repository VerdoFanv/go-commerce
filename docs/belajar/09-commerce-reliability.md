# 09 — Commerce + reliability (order, inventory, payment, outbox)

Tujuan: mereplikasi **problem bisnis sistem besar** — state, stok, bayar, event hilang, duplikat delivery — bukan cuma CRUD.

Buka sambil baca kode di tabel bawah.

---

## Problem yang sedang kamu pelajari

| Problem produksi | Solusi di repo ini |
|------------------|--------------------|
| Dual-write DB + Kafka | Transactional **outbox** |
| Kafka at-least-once → efek dobel | **Inbox** `(event_id, consumer)` + unique payment key |
| Oversell concurrent | `UPDATE stock … WHERE stock >= qty` |
| Inventori tanpa jejak | **stock_ledger** (hold/release/commit) |
| Status order liar | State machine `domain.CanTransition` |
| Broker down saat checkout | `POST /orders` tetap 201; pending di outbox |
| Hold stok menggantung (unpaid) | `ORDER_HOLD_TTL` + background expiry di API |
| Siapa yang charge? | Worker `payment` = canonical; `POST .../pay` = lab override |
| Observability lag event | Metric `golangbe_outbox_pending` + lab endpoints |

Yang **belum** (sengaja out of scope untuk lab mid-size): refund penuh, multi-warehouse, cart penuh, CDC/Debezium, payment gateway nyata, Kafka SASL.

---

## State machine

```text
pending_payment ──► paid ──► fulfilled
       │               ▲
       ├──► payment_failed ──┘ (retry pay)
       └──► cancelled
```

Kode: `internal/domain/order.go` — `CanTransition` / `Transition`.  
Handler **tidak** boleh `order.Status = "..."` bebas.

---

## Schema (migrations)

| Migration | Tabel |
|-----------|--------|
| `000005_commerce` | `orders`, `order_items`, `inventory_reservations`, `payments`, `idempotency_keys`, `outbox_events` |
| `000006_inbox_ledger` | `processed_events`, `stock_ledger` |
| `000007_inbox_composite` | PK `(event_id, consumer)` |

---

## Transactional outbox

| Langkah | Di mana |
|---------|---------|
| Insert order + hold + outbox row | **satu** TX — `order.Repository.CreateOrder` |
| Publish Kafka | `outbox.Relay` di `cmd/api` (bukan request path) |
| Kafka down | Order tetap sukses; `GET /lab/outbox/pending` |

Bandingkan dengan `product.Service.publishAsync` (fire-and-forget) — itu sengaja dibiarkan sebagai contoh anti-pattern untuk path kritis.

---

## Inbox + ledger

| Pattern | Kode | Fungsi |
|---------|------|--------|
| Inbox | `platform/inbox` | Claim per `(event_id, consumer)` sebelum side effect |
| Ledger | `platform/ledger` | Append movement: hold (−), release (+), commit (0) |
| Outbox lag | `metrics.OutboxPending` | Gauge unpublished rows |
| Hold TTL | `order.Service.RunHoldExpiry` | Cancel unpaid + release stock |

---

## HTTP API

| Method | Path | Wajib |
|--------|------|-------|
| POST | `/orders` | `Idempotency-Key` |
| GET | `/orders`, `/orders/:id` | JWT |
| POST | `/orders/:id/cancel` | dari `pending_payment` / `payment_failed` |
| POST | `/orders/:id/pay` | body `outcome`: success / fail / timeout (**lab**; worker = canonical) |
| POST | `/orders/:id/fulfill` | **admin** JWT; dari `paid` |

---

## Choreography

```text
order.created
  → payment worker (inbox)
      → payments row + status paid|payment_failed
      → outbox order.paid | order.payment_failed
order.paid
  → inventory commit (inbox + ledger)
order.cancelled | payment_failed
  → inventory release
* → audit Mongo
```

---

## Failure matrix (hidup)

`GET /api/v1/lab/commerce/failure-matrix`

Chaos outbox:

```bash
HOST=http://192.168.0.155   # atau localhost
APIKEY='apikey: lab-api-key-change-in-prod'
# login **admin** untuk lab chaos (RequireRole admin)
# login buyer → TOKEN=... untuk POST /orders
ADMIN_TOKEN=...
TOKEN=...

curl -s -X POST $HOST/api/v1/lab/outbox/pause -H "$APIKEY" -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s -X POST $HOST/api/v1/orders \
  -H "$APIKEY" -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: chaos-$(date +%s)" -H 'Content-Type: application/json' \
  -d '{"items":[{"productId":1,"qty":1}]}'
curl -s $HOST/api/v1/lab/outbox/pending -H "$APIKEY" -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s -X POST $HOST/api/v1/lab/outbox/resume -H "$APIKEY" -H "Authorization: Bearer $ADMIN_TOKEN"
curl -s -X POST $HOST/api/v1/lab/outbox/relay-once -H "$APIKEY" -H "Authorization: Bearer $ADMIN_TOKEN"
```

---

## Load / concurrency

```bash
chmod +x scripts/load-orders.sh scripts/bench.sh scripts/chaos-verify.sh
API=... API_KEY=... TOKEN=... PRODUCT_ID=1 N=20 ./scripts/load-orders.sh
# Pastikan stock produk tidak negatif di Postgres

HOST=http://192.168.0.155 API_KEY=... ./scripts/bench.sh      # capacity matrix
HOST=... API_KEY=... ./scripts/chaos-verify.sh                 # failure claims
```

Capacity: [`../BENCHMARK.md`](../BENCHMARK.md) · Failure: [`../FAILURE-RUNBOOK.md`](../FAILURE-RUNBOOK.md) · Modul: [10-failure-ops.md](10-failure-ops.md)

API clients: OpenAPI [`../openapi.yaml`](../openapi.yaml) · Postman [`../postman/`](../postman/)

---

## File map

| Path | Fungsi |
|------|--------|
| `migrations/000005_*.sql`, `000006_*.sql` | Schema |
| `internal/domain/order.go` | Status + events + transitions |
| `internal/platform/outbox/` | Writer + Relay |
| `internal/platform/inbox/` | Consumer dedupe |
| `internal/platform/ledger/` | Stock movements |
| `internal/http/order/` | API create/list/cancel/pay/fulfill |
| `internal/worker/dispatch/` | Route event |
| `internal/worker/payment/` | Simulator charge |
| `internal/worker/inventory/` | Commit/release |
| `internal/worker/audit/` | Mongo + DLQ |
| `test/unit/order`, `test/integration/order_api_test.go` | Spec |

---

## Latihan (checklist)

- [ ] Create order tanpa `Idempotency-Key` → 400  
- [ ] Create dua kali key sama → id order sama  
- [ ] Pause outbox (**admin**) → create → pending ≥ 1 → resume/relay → status jadi `paid`  
- [ ] `POST .../pay` `{outcome:fail}` pada order lain → `payment_failed` + stock kembali  
- [ ] `fulfill` sebagai **admin** dari `paid` → `fulfilled`; buyer → 403; dari `pending_payment` → 409  
- [ ] Tunggu / turunkan `ORDER_HOLD_TTL` → unpaid order auto-cancel + stock kembali  
- [ ] Baca `stock_ledger` untuk order yang di-hold  
- [ ] Buyer: `GET /products/catalog` melihat katalog seller  

Sebelumnya: [08-alur-end-to-end.md](08-alur-end-to-end.md) · Lanjut: [10-failure-ops.md](10-failure-ops.md) · Index: [README.md](README.md)
