# Failure & ops runbook

Honest matrix: what dies, what users see, what self-heals, what you must do.
Paired with lab chaos: `scripts/chaos-verify.sh` and Postman folder **5. Lab / chaos**.

Related: [BENCHMARK.md](BENCHMARK.md) · [belajar/10-failure-ops.md](belajar/10-failure-ops.md) · [openapi.yaml](openapi.yaml)

---

## Readiness model (API)

| Check | Critical? | Ready HTTP |
|-------|-----------|------------|
| Postgres | **yes** | down → **503** (pod drained) |
| Redis | **yes** | down → **503** (auth + RL need it) |
| Mongo | no | down → **200 degraded** (API commerce OK; audit lags on worker) |
| Typesense | no | down → **200 degraded** (search → 503 via CB; CRUD OK) |

`GET /health/live` always 200 if process is up.

---

## Dependency matrix

| Failure | User / API impact | Self-heal? | Operator steps | Signals |
|---------|-------------------|------------|----------------|---------|
| **Postgres down** | All CRUD/orders **500**; ready **503** | Pod CrashLoop if boot fails; else NotReady | Restore PG; wait pods Ready | ready checks.postgres; pod events |
| **Redis down** | Ready **503**. Auth RL **503**; login/refresh **500**. Non-auth RL fail-open **in-process** but ingress already drained | No traffic until Redis up | `docker compose -f infra-db up -d redis`; rollout if needed | ready redis; auth 503 |
| **Kafka down** | **Orders still 201** (outbox). Product events may be **lost** (async publish). Worker/notifier **retry with backoff** | Outbox drains when broker returns. Product path does **not** outbox | Bring Kafka up; watch `golangbe_outbox_pending` → 0 | outbox_pending; `outbox relay cycle failed` logs |
| **Mongo down** | API ready **degraded** (still Serving). Worker audit retries → DLQ after 3× | Redelivery until commit; DLQ for poison | Restore Mongo; check DLQ topic + `lab/mongo/events` | worker logs; DLQ |
| **Typesense down** | Ready **degraded**. `GET /products/search` **503** (CB). Product CRUD **201** | CB half-open after 30s | Start Typesense; optional `POST /lab/typesense/reindex` | `circuit breaker state change` logs |
| **Typesense up, index kosong** | Search kosong / (versi lama) **503** `Collection not found` | API OnStart: ensure + reindex jika `numDocuments=0` | Restart API; atau `POST /lab/typesense/reindex`. Compose `(unhealthy)` sering false alarm — cek `curl :8108/health` | `/collections` → `[]` / docs=0 |
| **API pod crash** | Brief errors; other replicas serve. In-memory outbox pause resets (relay resumes) | **k3s restart** | `kubectl -n golang-be get pods` | RestartCount |
| **Worker pod crash** | Orders stay `pending_payment` until redelivery | **k3s restart** + Kafka rebalance | Delete pod / wait rollout | orders stuck; kafka lag |
| **Outbox paused** (lab) | Orders 201; events pile up; no payment progress | **No** — manual resume | `POST /lab/outbox/resume` (+ `relay-once`) | `lab/outbox/pending` paused:true |

---

## Observability checklist

```bash
# Ready / degraded
curl -s $HOST/health/ready | jq .

# Metrics
curl -s $HOST/metrics | grep -E 'golangbe_outbox_pending|golangbe_events_|golangbe_http_requests_total'

# Lab (admin JWT, non-prod)
curl -s $HOST/api/v1/lab/overview -H "apikey: $API_KEY" -H "Authorization: Bearer $ADMIN"
curl -s $HOST/api/v1/lab/outbox/pending -H "apikey: $API_KEY" -H "Authorization: Bearer $ADMIN"
curl -s "$HOST/api/v1/lab/mongo/events?limit=5" -H "apikey: $API_KEY" -H "Authorization: Bearer $ADMIN"
```

Grafana (optional `make obs-up`): RPS, p95, publish/consume, DLQ, outbox pending.

---

## Standard recovery playbooks

### A. Orders stuck `pending_payment`

1. `GET /lab/outbox/pending` — unpublished? → resume/relay or fix Kafka.
2. Worker pods Running? → `kubectl -n golang-be logs -l app=worker --tail=100`.
3. Kafka healthy? → compose logs kafka.
4. After fix: poll `GET /orders/:id` until `paid` / `payment_failed`.

### B. Search empty / 503

1. Ready typesense check.
2. CB open? wait 30s or restart api after Typesense up.
3. `POST /lab/typesense/reindex` (admin).

### C. Auth suddenly 503

1. Redis down → restore Redis (critical ready).
2. Rate limit trip → wait window or raise `RATE_LIMIT_MAX` (lab only).

### D. Stock looks wrong after orders

1. Hold invalidates `product:{id}` cache (post-fix). Still: `DELETE /lab/redis/product/:id`.
2. Check `stock_ledger` + reservations in Postgres.

---

## What we do **not** claim

- Multi-AZ / multi-region failover.
- Product Kafka path is **not** dual-write safe (unlike orders/outbox).
- Outbox pause is shared via Redis (`outbox:relay:paused`) so **all API replicas** honor lab chaos.
  Falls back to in-memory if Redis set/get fails.
- Failure-matrix lab endpoint is **static expectations**, not a live probe.

---

## Verify on lab

```bash
HOST=http://192.168.0.155 API_KEY=lab-api-key-change-in-prod \
  ./scripts/chaos-verify.sh

# After ANY manual stop/start chaos (Typesense/Kafka/Redis/…), always restore:
HOST=http://192.168.0.155 API_KEY=lab-api-key-change-in-prod \
  SUDO_PASS='…' ./scripts/lab-restore.sh   # SUDO_PASS if k3s kubectl needs sudo
```

Or import Postman: [postman/](postman/) → env Lab → folder 5.

### Restore checklist (wajib setelah chaos)

| Yang sempat dimatikan / diubah | Restore |
|--------------------------------|---------|
| Typesense / Kafka | `docker compose up -d kafka typesense` (atau `./scripts/lab-restore.sh`) |
| Postgres / Redis / Mongo | `cd ~/projects/infra-db && docker compose up -d` |
| Outbox pause | `POST /lab/outbox/resume` (+ Redis `DEL outbox:relay:paused`) — `chaos-verify` EXIT trap juga melakukan ini |
| `RATE_LIMIT_MAX` dinaikkan untuk bench | `lab-restore.sh` mengembalikan ke `100` + rollout api |
| Typesense index kosong | `POST /lab/typesense/reindex` (admin) |
| Ready masih `degraded` | tunggu CB half-open (~30s) atau `kubectl -n golang-be rollout restart deploy/api` |

`chaos-verify.sh` **tidak** menghentikan container; ia hanya pause outbox lalu selalu resume di EXIT.  
Manual chaos (stop container) → **harus** diakhiri dengan `lab-restore.sh`.
