# 06 — HTTP features (auth, product, wishlist, lab, notify)

Tujuan: tiap package fitur — **route, fungsi service, side effect infra**.

Prefix semua REST di bawah: `/api/v1` (+ header `apikey`).

---

## Auth — `internal/http/auth/`

### Routes

| Method | Path | JWT | Handler |
|--------|------|-----|---------|
| POST | `/authentication/register` | tidak | `register` |
| POST | `/authentication/login` | tidak | `login` |
| POST | `/authentication/refresh-token` | tidak (body refresh) | `refresh` |
| GET | `/authentication/me` | ya | `me` |

### Layer

| File | Isi penting |
|------|-------------|
| `model.go` | `UserModel` → tabel `users` |
| `repository.go` | `Create`, `FindByEmail`, `FindByID` (+ unique email) |
| `service.go` | bcrypt hash, issue access/refresh JWT, map ke `domain.User` |
| `handler.go` | Bind JSON, `mapErr` → status |

### Service functions

| Fungsi | Apa yang dilakukan |
|--------|-------------------|
| `Register` | Validasi → hash password → insert → tokens |
| `Login` | Cari email → compare bcrypt → tokens |
| `Refresh` | Parse refresh JWT → issue pasangan token baru |
| `Me` | Load user by id dari access token |
| `issueTokens` | Access TTL pendek + refresh TTL panjang |

---

## Product — `internal/http/product/`

Ini fitur “paling lengkap”: Postgres + Redis cache + Kafka publish + Typesense index.

### Routes (semua JWT)

| Method | Path | Handler |
|--------|------|---------|
| GET | `/products` | `list` (cursor pagination) |
| GET | `/products/search` | `search` (Typesense) — **sebelum** `/:id` |
| POST | `/products` | `create` |
| GET | `/products/:id` | `getByID` |
| PUT | `/products/:id` | `update` |
| DELETE | `/products/:id` | `delete` (owner atau admin di service) |

### Interfaces di service

| Interface | Implementasi wire | Fungsi |
|-----------|-------------------|--------|
| `EventPublisher` | Kafka producer | Publish create/update/delete |
| `SearchEngine` | Typesense client | Index / delete / search |

### Service functions (inti)

| Fungsi | Postgres | Redis | Kafka | Typesense |
|--------|----------|-------|-------|-----------|
| `Create` | insert | invalidate list | `product.created` async | index async |
| `GetByID` | miss → DB | get/set `product:{id}` | — | — |
| `List` | cursor query | cache first page | — | — |
| `Update` | update owner | invalidate | `product.updated` | reindex |
| `Delete` | delete + RBAC | invalidate | `product.deleted` | delete doc |
| `Search` | — | — | — | query |

Helper cache: `getCache`, `setCache`, `deleteCache`, `invalidateListCache`, `publishAsync`, `indexAsync`, …

---

## Wishlist — `internal/http/wishlist/`

### Routes (JWT)

| Method | Path | Handler |
|--------|------|---------|
| GET | `/wishlists` | `list` |
| GET | `/wishlists/count` | `count` (Redis-backed) |
| POST | `/wishlists` | `add` |
| DELETE | `/wishlists/:id` | `remove` |

### Service

| Fungsi | Perilaku |
|--------|----------|
| `Add` | Cek product ada (`ProductFinder`) → insert → invalidate count cache |
| `List` | Join/enrich nama+harga product |
| `Remove` | Hapus milik user → invalidate count |
| `Count` | Redis hit, else DB count + set cache |

Belajar pola: **counter hot path di Redis**, source of truth tetap Postgres.

---

## Lab — `internal/http/lab/`

Endpoint **belajar infra**, bukan domain bisnis production. Tetap butuh JWT + apikey.

| Method | Path | Fungsi service |
|--------|------|----------------|
| GET | `/lab/overview` | Status ringkas semua dependency |
| GET | `/lab/postgres/summary` | Count tabel |
| GET | `/lab/postgres/samples` | Sample products |
| GET | `/lab/redis/product/:id` | Peek cache product |
| DELETE | `/lab/redis/product/:id` | Invalidate cache |
| GET | `/lab/mongo/events` | Recent audit records |
| POST | `/lab/kafka/ping` | Publish `lab.ping` |
| GET | `/lab/typesense` | Explore/search + stats |
| POST | `/lab/typesense/reindex` | Rebuild index dari Postgres |

Pakai ini untuk verifikasi end-to-end tanpa harus “nebak” dari log saja.

---

## Notify — `internal/http/notify/`

Bukan CRUD; real-time fan-out.

| Symbol | Fungsi |
|--------|--------|
| `Hub` | Set koneksi WS; `Add`/`Remove`/`Broadcast`/`Count` |
| `Notifier.Run` | Loop `Fetch` Kafka (group notifier) → `Hub.Broadcast` → `Commit` |
| `Handler.products` | Upgrade HTTP → WebSocket `/ws/products` |
| `Handler.authenticate` | Validasi JWT sebelum upgrade |

Client: connect dengan token → create/update product di terminal lain → event muncul di WS.

---

## Pola `mapErr` di setiap handler

Hampir semua feature punya `mapErr(c, err)`:

- `domain.Err*` → status HTTP + `FailCode`
- error validasi bind → 400
- sisanya → 500 (jangan bocorkan detail internal ke client)

Tiru file auth/product kalau menambah feature baru.

Lanjut → [07-worker.md](07-worker.md)
