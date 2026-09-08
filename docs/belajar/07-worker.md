# 07 — Worker audit (`internal/worker/audit/`)

Tujuan: paham proses kedua — **consume Kafka → tulis Mongo → commit / DLQ**.

Buka: `cmd/worker/main.go`, `internal/worker/audit/audit.go`, `store.go`.

---

## Kenapa worker terpisah dari API?

| | API | Worker |
|-|-----|--------|
| Scaling | HTTP RPS | Lag Kafka / throughput write |
| Failure | User langsung lihat error | Retry + DLQ tanpa blokir request |
| Deploy | Rolling + readiness | Bisa restart tanpa drop traffic HTTP |

Satu topic, consumer group **beda** dari notifier di API.

---

## Tipe inti

### `Record` (dokumen Mongo)

| Field | Arti |
|-------|------|
| `eventId` | ID event Kafka (unique index → idempotent) |
| `type` | mis. `product.created`, `lab.ping` |
| `payload` | isi event |
| `occurredAt` / `processedAt` | waktu event vs waktu audit |
| `consumer` | `"worker"` |

### `Store` interface

`Insert` saja di interface processor — mudah di-fake di unit test.  
`MongoStore` menambah `ListRecent`, `Count`, `EnsureIndexes` untuk lab + boot.

---

## `Processor.Handle` — alur satu message

```text
Fetch message
    │
    ├─ event.ID kosong? → deadLetter (poison) → return nil (caller commit)
    │
    └─ bangun Record
           │
           ├─ Insert Mongo (max 3x, backoff exponential)
           │     sukses → return nil → caller Commit offset
           │
           └─ gagal semua → deadLetter → return nil → commit
                (pesan “selesai” dari sudut consumer; salinan di DLQ)
```

| Fungsi | Fungsi bisnis |
|--------|----------------|
| `Handle` | Persist + retry + DLQ decision |
| `deadLetter` | Publish ke topic DLQ dengan `reason` + `rawMessage` |

**Penting:** loop di `cmd/worker` hanya `Commit` kalau `Handle` return `nil`. Kalau `Handle` return error (mis. DLQ publish gagal), offset tidak maju → message di-retry.

---

## `MongoStore`

| Fungsi | Fungsi |
|--------|--------|
| `NewMongoStore` | Collection audit di DB config |
| `Insert` | Insert satu `Record` |
| `EnsureIndexes` | Unique `eventId` |
| `ListRecent` / `Count` | Dipakai lab API |

Index unique `eventId` mencegah dokumen audit dobel. Saat ini `Insert` **meneruskan** error duplicate key ke caller — artinya re-delivery bisa masuk retry lalu DLQ. Saat belajar, bandingkan komentar `EnsureIndexes` dengan perilaku `Insert`; perbaikan umum di industri: treat duplicate key sebagai sukses (idempotent ack).

---

## Metrics worker

Proses worker expose `/metrics` di `METRICS_PORT` (bukan port HTTP API).  
Counter event consumed / dead-lettered diisi dari path publish/consume (lihat pemanggilan `metrics.*` di codebase).

---

## Latihan praktek

1. `POST /api/v1/lab/kafka/ping` → log worker `audited` → `GET /lab/mongo/events`.
2. Matikan Mongo sebentar → lihat retry / DLQ di log.
3. Bandingkan group id worker vs notifier di config.

Lanjut → [08-alur-end-to-end.md](08-alur-end-to-end.md)
