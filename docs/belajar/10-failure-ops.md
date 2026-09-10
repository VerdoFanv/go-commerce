# 10 — Failure, ops & real chaos

Tujuan: berpikir seperti developer on-call. Spek bilang “bisa degrade / self-heal”?
**Jangan percaya README — buktikan.**

Prasyarat: modul [09](09-commerce-reliability.md), stack lab hidup, token **admin**.

Ops ringkas (EN/ID mix OK): [`../FAILURE-RUNBOOK.md`](../FAILURE-RUNBOOK.md) · capacity: [`../BENCHMARK.md`](../BENCHMARK.md)

---

## Pertanyaan yang harus kamu jawab

1. Infra X mati — request apa yang masih 2xx? apa yang 5xx/503?
2. Sistem **self-heal** atau butuh tangan manusia?
3. Log / metric / lab endpoint mana yang menangkap masalah?
4. Dampak ke user (checkout, login, search, WS)?
5. Setelah recover — data konsisten? (stock, outbox, audit)

---

## Readiness: critical vs optional

Kode: `internal/http/health/`.

| Dep | Critical | Ready |
|-----|----------|-------|
| Postgres, Redis | ya | 503 → pod keluar Service |
| Mongo, Typesense | tidak | 200 `degraded` + detail checks |

Ini sengaja: spek “CRUD survive Typesense down” **harus** tetap Serving.

Latihan:

```bash
curl -s $HOST/health/live
curl -s $HOST/health/ready | jq .
# stop typesense → ready message=degraded, mode=degraded, typesense down
# catalog / orders masih jalan; search → 503
```

---

## Eksperimen wajib (chaos)

Script: `./scripts/chaos-verify.sh` (butuh admin login).  
**Setelah selesai / setelah stop container manual:** `./scripts/lab-restore.sh` (wajib).

`chaos-verify` memasang `trap EXIT` → outbox selalu di-resume meski script gagal di tengah.

### 1) Outbox pause ≈ Kafka publish stall

| Langkah | Expect |
|---------|--------|
| `POST /lab/outbox/pause` | paused |
| `POST /orders` + Idempotency-Key | **201** |
| `GET /lab/outbox/pending` | count ≥ 1 |
| `resume` + `relay-once` | pending turun; order → `paid` |
| Metric `golangbe_outbox_pending` | naik lalu turun |

**Self-heal:** setelah resume, relay loop otomatis. Pause sendiri **tidak** auto-resume.

### 2) Typesense down

| Langkah | Expect |
|---------|--------|
| Stop typesense | ready **degraded** |
| `GET /products/search?q=x` | 503 (circuit breaker) |
| `POST /products` | **201** |
| Start + reindex | search kembali |

Log: `circuit breaker state change`.

### 3) Redis down

| Langkah | Expect |
|---------|--------|
| Stop redis | ready **503** (critical) |
| Login via ingress | gagal / no endpoints |
| Restore redis | ready 200; login OK |

Auth RL fail-closed; refresh token juga butuh Redis.

### 4) Kafka down (orders vs products)

| Langkah | Expect |
|---------|--------|
| Stop kafka | `POST /orders` → **201**, outbox pending naik |
| `POST /products` | **201**, tapi event product bisa **hilang** (bukan outbox) |
| Start kafka | outbox drain; product event **tidak** replay otomatis |

### 5) Worker crash

```bash
sudo k3s kubectl -n golang-be delete pod -l app=worker
# create order → setelah pod Running, status jadi paid
# GET /lab/mongo/events — audit muncul
```

Worker fetch sekarang **retry + backoff** (bukan exit diam-diam).

### 6) Rate limit

`RATE_LIMIT_MAX=30` → hammer catalog → **429**, bukan 5xx. Restore limit setelah tes.

---

## Observability yang harus “nyala”

| Signal | Dimana |
|--------|--------|
| Ready / degraded | `/health/ready` |
| Outbox lag | `golangbe_outbox_pending`, `/lab/outbox/pending` |
| Publish / consume / DLQ | Prometheus counters |
| Audit | `/lab/mongo/events` |
| Cache stale | `/lab/redis/product/:id` |
| CB Typesense | API logs |

Kalau signal tidak bergerak saat chaos — itu bug observability, bukan “sistem sehat”.

---

## Checklist lulus modul

- [ ] Jelaskan kenapa Mongo down ≠ API NotReady (tapi audit bisa telat)
- [ ] Pause outbox → order 201 → resume → paid (bukti sendiri)
- [ ] Typesense down → degraded + search 503 + create product 201 → **`lab-restore.sh`**
- [ ] Bedakan jalur product (fire-and-forget) vs order (outbox)
- [ ] Tahu cara baca outbox_pending + mongo events + worker logs
- [ ] Tahu Postman folder Lab + env lab ([`../postman/`](../postman/))
- [ ] Lab kembali `message=ready` (bukan degraded) sebelum logout dari sesi belajar

Sebelumnya: [09-commerce-reliability](09-commerce-reliability.md) · Index: [README.md](README.md)
