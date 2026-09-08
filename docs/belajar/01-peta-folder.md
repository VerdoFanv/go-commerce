# 01 — Peta folder

Tujuan: tahu **di mana** mencari kode, tanpa hafal semua file.

---

## Dua proses, dua folder delivery

Repo ini jalan sebagai **dua binary**:

| Binary | Entry | Folder kode utamanya |
|--------|-------|----------------------|
| API | `cmd/api` | `internal/http/` |
| Worker | `cmd/worker` | `internal/worker/` |

Shared (dipakai keduanya): `internal/domain`, `internal/config`, `internal/platform`, `internal/metrics`.

```text
golang-be/
├── cmd/
│   ├── api/main.go          # composition root HTTP
│   └── worker/main.go       # composition root Kafka worker
├── internal/
│   ├── http/                ★ API only
│   │   ├── server/          # susun Gin + pasang route
│   │   ├── middleware/      # apikey, JWT, rate limit, ...
│   │   ├── health/          # /health/live|ready
│   │   ├── auth/            # register/login/refresh/me
│   │   ├── product/         # CRUD + cache + Kafka + search
│   │   ├── wishlist/        # wishlist + Redis count
│   │   ├── lab/             # endpoint belajar infra
│   │   └── notify/          # WebSocket hub + notifier consumer
│   ├── worker/              ★ worker only
│   │   └── audit/           # consume → Mongo + DLQ
│   ├── domain/              # entity + error bersama
│   ├── config/              # baca env
│   ├── platform/            # adapter DB/Redis/Kafka/...
│   └── metrics/             # Prometheus
├── pkg/response/            # envelope JSON API
├── migrations/              # SQL versioned
└── test/                    # unit + integration
```

---

## Pola feature HTTP (`internal/http/<nama>/`)

Hampir semua fitur API punya 4 file:

| File | Tanggung jawab |
|------|----------------|
| `handler.go` | Parse HTTP, panggil service, map error → status |
| `service.go` | Business rules |
| `repository.go` | Akses DB (GORM) saja |
| `model.go` | Struct tabel GORM |

**Jangan** taruh query SQL/GORM di handler. **Jangan** parse `fiber/gin.Context` di service.

---

## Apa yang TIDAK ada di `internal/http`

- Loop Kafka worker → itu `internal/worker/audit`
- Koneksi Postgres/Redis mentah → `internal/platform/*`
- Bentuk error/entity publik → `internal/domain`

---

## Checklist “file ini buat apa?”

| Kamu mau… | Buka… |
|-----------|--------|
| Tambah route baru | `internal/http/<feature>/handler.go` + `server/server.go` (kalau belum di-wire) |
| Ubah JWT / apikey | `internal/http/middleware/` |
| Ubah aturan “siapa boleh delete product” | `internal/http/product/service.go` |
| Ubah cara event di-audit | `internal/worker/audit/` |
| Ubah env / validasi boot | `internal/config/config.go` |
| Ubah koneksi Kafka | `internal/platform/kafka/kafka.go` |
| Ubah format JSON response | `pkg/response/` |
| Baca test sebagai contoh pemakaian | `test/unit/<feature>/`, `test/integration/` |

Lanjut → [02-entrypoints.md](02-entrypoints.md)
