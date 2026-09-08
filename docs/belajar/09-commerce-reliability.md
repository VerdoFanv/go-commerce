# 09 — Commerce + reliability (order, inventory, payment, outbox)

Tujuan: mereplikasi **problem bisnis sistem besar** — bukan hanya CRUD + infra.

Buka sambil baca: `internal/http/order/`, `internal/platform/outbox/`, `internal/worker/payment|inventory|dispatch/`.

---

## State machine order

```text
pending_payment ──► paid ──► fulfilled
       │               ▲
       ├──► payment_failed ──┘ (retry pay)
       └──► cancelled
```

`domain.CanTransition` / `Transition` — handler/service tidak set status bebas.

---

## Transactional outbox

| Langkah | Di mana |
|---------|---------|
| Insert order + hold stock + `outbox_events` | **satu** TX Postgres (`order.Repository.CreateOrder`) |
| Publish Kafka | `outbox.Relay` di proses API (bukan di request path) |
| Kafka down | `POST /orders` tetap 201; lihat `GET /lab/outbox/pending` |

Bandingkan dengan product `publishAsync` (fire-and-forget) — path kritis order **tidak** boleh kehilangan event.

---

## Inbox + stock ledger

| Pattern | Tabel / kode | Problem yang dipecahkan |
|---------|--------------|-------------------------|
| **Inbox** | `processed_events` + `platform/inbox` | At-least-once Kafka → side effect sekali per consumer |
| **Stock ledger** | `stock_ledger` + `platform/ledger` | Jejak hold/release/commit (audit inventori seperti sistem besar) |
| **Outbox lag metric** | `golangbe_outbox_pending` | Observability: relay tertinggal |

Fulfill: `POST /orders/:id/fulfill` (`paid` → `fulfilled`).


---

## Choreography

```text
order.created  → payment worker → outbox order.paid | order.payment_failed
order.paid     → inventory commit (idempotent)
order.cancelled / payment_failed → inventory release
*              → audit Mongo
```

Manual: `POST /orders/:id/pay` dengan `outcome=success|fail|timeout`.

---

## Failure matrix (hidup)

`GET /api/v1/lab/commerce/failure-matrix`

Chaos outbox:

```bash
POST /lab/outbox/pause    # relay berhenti → Kafka seolah down
POST /orders              # tetap sukses
GET  /lab/outbox/pending
POST /lab/outbox/resume
POST /lab/outbox/relay-once
```

---

## Load / concurrency

```bash
chmod +x scripts/load-orders.sh
API=... API_KEY=... TOKEN=... PRODUCT_ID=1 N=20 ./scripts/load-orders.sh
```

Stock di-hold dengan `UPDATE ... WHERE stock >= qty` — oversell → 409, stock tidak negatif.

---

## File penting

| Path | Fungsi |
|------|--------|
| `migrations/000005_commerce.*.sql` | Schema commerce |
| `internal/domain/order.go` | Status + events + transitions |
| `internal/platform/outbox/` | Writer + Relay |
| `internal/http/order/` | API create/list/get/cancel/pay |
| `internal/worker/dispatch/` | Route event → payment/inventory/audit |
| `internal/worker/payment/` | Simulator charge |
| `internal/worker/inventory/` | Commit/release reservation |
