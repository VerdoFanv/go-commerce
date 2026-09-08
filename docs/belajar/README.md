# Belajar kode Golang BE

Mulai dari sini. Panduan dipecah biar tidak numpuk — baca berurutan, buka file kode di editor sambil baca.

## Urutan baca (disarankan)

| # | File | Isi |
|---|------|-----|
| 1 | [01-peta-folder.md](01-peta-folder.md) | Peta `internal/` — mana API, mana worker, mana shared |
| 2 | [02-entrypoints.md](02-entrypoints.md) | `cmd/api` & `cmd/worker` — proses start, fx wiring, lifecycle |
| 3 | [03-shared-core.md](03-shared-core.md) | `domain`, `config`, `metrics`, `pkg/response`, migrations |
| 4 | [04-platform.md](04-platform.md) | Adapter infra: Postgres, Redis, Mongo, Kafka, Typesense, OTel |
| 5 | [05-http-server-middleware.md](05-http-server-middleware.md) | Gin server + rantai middleware |
| 6 | [06-http-features.md](06-http-features.md) | Auth, product, wishlist, lab, health, notify |
| 7 | [07-worker.md](07-worker.md) | Audit consumer, retry, DLQ, Mongo store |
| 8 | [08-alur-end-to-end.md](08-alur-end-to-end.md) | Satu request create product → Kafka → worker + WebSocket |

## Ops / lab (terpisah)

- Setup, deploy, infra, backup → [`../PANDUAN-BELAJAR.md`](../PANDUAN-BELAJAR.md)
- GitLab dual mode → [`../GITLAB-SETUP.md`](../GITLAB-SETUP.md)
- Public IP / domain → [`../PUBLIC-ACCESS.md`](../PUBLIC-ACCESS.md)

## Cara belajar yang efektif

1. Buka file Go yang disebut di panduan.
2. Cari symbol (fungsi/tipe) yang dijelaskan — jangan hanya baca markdown.
3. Ikuti **siapa memanggil siapa** (handler → service → repo / platform).
4. Setelah satu modul, coba ubah log / breakpoint kecil, jalankan test terkait.

```bash
# contoh: test auth saja
go test ./test/unit/auth/ ./test/integration/ -run Auth -count=1
```

## Cheat sheet 10 detik

```text
cmd/api          → proses HTTP (Gin)
cmd/worker       → proses Kafka consumer

internal/http/   → SEMUA kode API
internal/worker/ → SEMUA kode worker
internal/domain|config|platform|metrics → dipakai keduanya
```
