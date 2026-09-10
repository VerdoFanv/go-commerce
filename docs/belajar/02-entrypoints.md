# 02 — Entrypoints (`cmd/api` & `cmd/worker`)

Tujuan: paham **proses hidup** — siapa di-construct, kapan migrate, kapan relay/consume, kapan shutdown.

Kedua binary pakai **Uber fx**.

---

## `cmd/api/main.go` — proses API

### `main()`

1. `godotenv.Load()` (lokal; di k8s env sudah inject).
2. `fx.New(...).Run()` sampai SIGINT/SIGTERM.

### Graph (ringkas)

| Grup | Constructor | Hasil |
|------|-------------|--------|
| Config | `config.Load` | `Config` |
| Platform | `database`, `redis`, `mongo`, `typesense`, `telemetry` | client infra |
| Kafka | Producer (products topic), Consumer (notifier group) | publish + WS consume |
| Outbox | `outbox.NewWriter`, `outbox.NewRelay(db, producer)` | TX enqueue + poll publish |
| HTTP features | auth / product / wishlist / **order** / lab | handlers |
| Ops | health, notify hub/notifier | probes + WS |
| Server | `server.NewEngine`, `NewHTTPServer` | Gin |

### Lifecycle OnStart

1. `database.Migrate` — **hanya API** (`000001`…`000007`, termasuk commerce + inbox composite).
2. `kafka.EnsureTopics`.
3. `go notifier.Run` — group notifier → WebSocket.
4. `go relay.Run` — publish `outbox_events` yang belum `published_at`.
5. `go orderSvc.RunHoldExpiry` — cancel `pending_payment` lewat `ORDER_HOLD_TTL`, release stock.
6. `go ListenAndServe`.

OnStop: cancel notifier + relay + hold-expiry → `http.Shutdown` → close clients.

### Kenapa relay di API?

Order menulis outbox di Postgres. Relay harus jalan selama API hidup supaya event keluar ke Kafka. Worker **tidak** migrate dan tidak wajib punya relay (tapi menulis outbox payment hasil charge — API relay yang publish).

---

## `cmd/worker/main.go` — proses worker

### Graph

| Provide | Fungsi |
|---------|--------|
| `database.Connect` | Postgres untuk payment/inventory/inbox/ledger |
| `mongo` → `audit.MongoStore` | audit trail |
| `outbox.NewWriter` | payment enqueue `order.paid` / `payment_failed` |
| `payment.NewHandler`, `inventory.NewHandler` | side effects bisnis |
| `kafka.Consumer` (worker group) + DLQ `Producer` | consume / dead-letter |
| `audit.NewProcessor` + `dispatch.NewProcessor` | route lalu audit |

### Loop

```text
Fetch → dispatch.Handle (commerce dulu, lalu audit) → Commit jika nil
```

`dispatch` memanggil:

- `order.created` → payment (inbox claim → charge → outbox)
- `order.paid` / `cancelled` / `payment_failed` → inventory (inbox + ledger)
- selalu → audit Mongo (duplicate `eventId` = sukses)

### At-least-once

Offset commit **setelah** handler sukses. Redelivery aman karena inbox + unique payment key + status reservation.

---

## Latihan

1. Cocokkan `fx.Provide` di `cmd/api` dengan tabel di atas (cari `order.`, `outbox.`).
2. Pause relay lewat lab — create order — pastikan pending outbox naik.
3. Bandingkan group: `KafkaGroupNotifier` vs `KafkaGroupWorker`.

Lanjut → [03-shared-core.md](03-shared-core.md)
