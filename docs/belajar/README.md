# Belajar kode Golang BE

Mulai dari sini. Panduan **dipisah per topik** biar enak dibaca — buka file Go di editor sambil ikut urutan.

Ada **dua jalur** (boleh campur):

| Jalur | Fokus | Modul |
|-------|--------|--------|
| **A — Fondasi** | Struktur, HTTP, product CRUD, infra adapters | 01 → 08 |
| **B — Sistem besar** | Order, outbox, inbox, payment, inventory, failure matrix | **09** (+ update di 02/04/06/07/08) |
| **C — On-call** | Infra mati, self-heal, observability, chaos nyata | **10** + [`FAILURE-RUNBOOK`](../FAILURE-RUNBOOK.md) |

Kalau goal-mu mereplikasi problem bisnis produksi: **baca 01–04 singkat, lalu loncat ke 09**, setelah itu **10** untuk buktikan spek failure, lalu balik ke 07–08 untuk E2E.

---

## Urutan baca

| # | File | Isi | Update terbaru |
|---|------|-----|----------------|
| 1 | [01-peta-folder.md](01-peta-folder.md) | Peta `internal/` API vs worker vs shared | + `order`, `dispatch`, outbox/inbox/ledger |
| 2 | [02-entrypoints.md](02-entrypoints.md) | `cmd/api` & `cmd/worker` + fx lifecycle | + outbox relay, **hold expiry**, worker dispatch |
| 3 | [03-shared-core.md](03-shared-core.md) | domain, config, metrics, response, migrations | + `000007` inbox composite, CORS/HSTS/hold TTL |
| 4 | [04-platform.md](04-platform.md) | Adapter infra | + **outbox**, **inbox**, **ledger** |
| 5 | [05-http-server-middleware.md](05-http-server-middleware.md) | Gin + middleware | ready critical/optional, security |
| 6 | [06-http-features.md](06-http-features.md) | Auth, product, wishlist, **order**, lab, notify | logout/rotate, catalog, lab admin |
| 7 | [07-worker.md](07-worker.md) | Worker penuh | dispatch → payment / inventory / audit + **fetch retry** |
| 8 | [08-alur-end-to-end.md](08-alur-end-to-end.md) | Cerita request | product **dan** order E2E |
| 9 | [09-commerce-reliability.md](09-commerce-reliability.md) | Lab problem sistem besar | hold TTL, inbox, payment ownership |
| 10 | [10-failure-ops.md](10-failure-ops.md) | Chaos infra + self-heal + signals | readiness degraded, chaos script |

Ops / deploy / API clients:

- [`../PANDUAN-BELAJAR.md`](../PANDUAN-BELAJAR.md)
- [`../FAILURE-RUNBOOK.md`](../FAILURE-RUNBOOK.md) — apa yang mati, dampak, recovery
- [`../BENCHMARK.md`](../BENCHMARK.md) — capacity SLI
- [`../openapi.yaml`](../openapi.yaml) + [`../postman/`](../postman/) — kontrak + Postman collection/env
- [`../GITLAB-SETUP.md`](../GITLAB-SETUP.md)
- [`../PUBLIC-ACCESS.md`](../PUBLIC-ACCESS.md)
- Ringkasan portfolio (EN): [`../../README.md`](../../README.md)

---

## Cara belajar yang efektif

1. Buka file Go yang disebut — jangan cuma scroll markdown.
2. Trace **siapa memanggil siapa** (handler → service → repo / platform).
3. Bandingkan dua jalur publish: `product.publishAsync` vs `outbox.Relay`.
4. Praktek lab: `POST /orders` → pause outbox → pending → resume → worker `paid`.
5. Jalankan chaos: `HOST=... API_KEY=... make chaos` (Litmus) dan/atau `make chaos-outbox`.

```bash
go test ./test/unit/order/ ./test/unit/domain/ ./test/integration/ -run Order -count=1
go test ./test/unit/audit/ ./test/unit/payment/ ./test/unit/inventory/ ./internal/http/health/ -count=1
```

## Cheat sheet

```text
cmd/api      → HTTP + migrate + outbox relay + WS notifier (retry)
cmd/worker   → dispatch(payment|inventory) + audit + DLQ (fetch retry)

internal/http/order     → commerce API
internal/http/health    → live / ready (critical vs optional)
internal/platform/outbox|inbox|ledger → reliability patterns
internal/worker/dispatch|payment|inventory|audit
```
