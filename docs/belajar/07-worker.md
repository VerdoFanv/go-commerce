# 07 — Worker (dispatch, payment, inventory, audit)

Tujuan: paham proses kedua — **consume Kafka → side effect bisnis → audit → commit / DLQ**.

Buka: `cmd/worker/main.go`, `internal/worker/dispatch/`, `payment/`, `inventory/`, `audit/`.

---

## Kenapa worker terpisah?

| | API | Worker |
|-|-----|--------|
| Scaling | HTTP RPS | Lag Kafka / throughput write |
| Failure | User lihat error | Retry + DLQ tanpa blokir request |
| Deploy | Rolling + readiness | Restart tanpa drop traffic HTTP |

Satu topic `products.events`, consumer group **beda** dari notifier di API.

---

## `dispatch.Processor`

```text
Fetch message
    │
    ├─ routeCommerce (jika event.ID ada)
    │     order.created              → payment.HandleOrderCreated
    │     order.paid|cancelled|…     → inventory.Handle
    │
    └─ audit.Handle  (selalu; poison → DLQ)
           │
           sukses → caller Commit offset
```

File: `internal/worker/dispatch/dispatch.go`.

---

## Payment — `internal/worker/payment/`

Pada `order.created`:

1. `inbox.Claim(tx, "payment", eventID)` — duplikat → skip
2. Lock order row
3. Simulasikan charge (default success; payload bisa `simulateOutcome`)
4. Insert `payments` (unique `order_id + idempotency_key`)
5. Transition status + commit/release reservation + **ledger**
6. Enqueue outbox `order.paid` atau `order.payment_failed`

Manual alternatif dari API: `POST /orders/:id/pay`.

---

## Inventory — `internal/worker/inventory/`

| Event | Efek |
|-------|------|
| `order.paid` / `fulfilled` | Commit reservation `held` → `committed` (+ ledger `commit`) |
| `order.cancelled` / `payment_failed` | Release: stock += qty, status `released` (+ ledger `release`) |

Idempotent: inbox + hanya baris berstatus `held` yang diubah.

---

## Audit — `internal/worker/audit/`

Tetap pola lama, diperkeras:

| Symbol | Fungsi |
|--------|--------|
| `Record` | Dokumen Mongo (`eventId`, type, payload, …) |
| `Handle` | Retry insert 3x → DLQ |
| `MongoStore.Insert` | **Duplicate `eventId` = sukses** (idempotent ack) |

Poison (ID kosong) → DLQ tanpa buang retry Mongo.

---

## At-least-once checklist

Fetch gagal (broker blip) → **retry + exponential backoff** (bukan exit diam-diam meninggalkan metrics server hidup tanpa consumer). Lihat loop di `cmd/worker/main.go`. Notifier API (`notify.Hub`) sama.

---

## At-least-once + idempotency

1. Commit Kafka offset **hanya** jika `dispatch.Handle` return nil.
2. Crash setelah side effect + sebelum commit → redelivery.
3. Inbox / unique keys / reservation status mencegah efek dobel.

---

## Latihan

1. `POST /lab/kafka/ping` → log worker → `GET /lab/mongo/events`.
2. Create order → tunggu status `paid` → cek `stock_ledger` di Postgres.
3. Kill worker mid-flight → pastikan order tetap konsisten setelah restart.
4. Stop Kafka sebentar → pastikan worker log `will retry` lalu recover (modul [10](10-failure-ops.md)).

Lanjut → [08-alur-end-to-end.md](08-alur-end-to-end.md)
