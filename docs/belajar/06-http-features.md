# 06 — HTTP features (auth, product, wishlist, order, lab, notify)

Tujuan: tiap package fitur — **route, fungsi service, side effect infra**.

Prefix REST: `/api/v1` (+ header `apikey`).

---

## Auth — `internal/http/auth/`

### Routes

| Method | Path | JWT | Handler |
|--------|------|-----|---------|
| POST | `/authentication/register` | tidak | `register` (password **min 8**) |
| POST | `/authentication/login` | tidak | `login` |
| POST | `/authentication/refresh-token` | tidak (body refresh) | `refresh` — **rotasi** (jti Redis) |
| POST | `/authentication/logout` | tidak (body refresh) | `logout` — revoke jti |
| GET | `/authentication/me` | ya | `me` |

### Layer

| File | Isi penting |
|------|-------------|
| `model.go` | `UserModel` → tabel `users` |
| `repository.go` | `Create`, `FindByEmail`, `FindByID` (+ unique email) |
| `service.go` | bcrypt, issue tokens, store/consume refresh `jti` di Redis |
| `handler.go` | Bind JSON, `mapErr` → status |

### Service functions

| Fungsi | Apa yang dilakukan |
|--------|-------------------|
| `Register` | Validasi (≥8) → hash → insert → tokens |
| `Login` | Cari email → compare bcrypt → tokens |
| `Refresh` | Parse refresh (HMAC + type) → **hapus jti lama** → issue pasangan baru |
| `Logout` | Hapus jti refresh dari Redis |
| `Me` | Load user by id dari access token |
| `issueTokens` | Access TTL pendek + refresh TTL panjang + `jti` |

---

## Product — `internal/http/product/`

Ini fitur “paling lengkap”: Postgres + Redis cache + Kafka publish + Typesense index.

### Routes (semua JWT)

| Method | Path | Handler |
|--------|------|---------|
| GET | `/products` | `list` (milik seller / user sendiri, cursor) |
| GET | `/products/catalog` | `catalog` (**buyer browse** semua seller) |
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
| `Catalog` | semua seller, cursor | — | — | — |
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

## Order — `internal/http/order/` (commerce)

Ini jalur **reliability**: transactional outbox + stock hold + idempotency. Detail mendalam → [09-commerce-reliability.md](09-commerce-reliability.md).

### Routes (JWT)

| Method | Path | Catatan |
|--------|------|---------|
| POST | `/orders` | **Wajib** header `Idempotency-Key` |
| GET | `/orders` | List milik user |
| GET | `/orders/:id` | Detail + items |
| POST | `/orders/:id/cancel` | State machine + release stock + outbox |
| POST | `/orders/:id/pay` | Lab override simulator (`success\|fail\|timeout`); **canonical charge = worker** |
| POST | `/orders/:id/fulfill` | **admin only** — `paid` → `fulfilled` + outbox |

### Create (inti)

Satu TX Postgres:

1. `UPDATE products SET stock = stock - qty WHERE stock >= qty` (anti oversell)
2. Insert `orders` + `order_items` + `inventory_reservations` (`held`)
3. `stock_ledger` reason `hold`
4. `outbox_events` type `order.created`
5. Setelah sukses: simpan response di `idempotency_keys` untuk replay

**Bukan** `go kafka.Publish` di request path.

### Bandingkan dengan product

| | Product | Order |
|-|---------|-------|
| Publish | `publishAsync` setelah commit | Outbox dalam TX yang sama |
| Kafka down | Event bisa hilang | Order tetap 201; pending di outbox |
| Idempotency HTTP | tidak | `Idempotency-Key` |

---

## Lab — `internal/http/lab/`

Endpoint **belajar infra + reliability**.

- **Non-production only** — tidak di-register saat `APP_ENV=production`.
- Middleware: JWT + **`RequireRole(admin)`**.

| Method | Path | Fungsi service |
|--------|------|----------------|
| GET | `/lab/overview` | Status ringkas semua dependency |
| GET | `/lab/postgres/summary` | Count tabel (+ orders) |
| GET | `/lab/postgres/samples` | Sample products |
| GET | `/lab/redis/product/:id` | Peek cache product |
| DELETE | `/lab/redis/product/:id` | Invalidate cache |
| GET | `/lab/mongo/events` | Recent audit records |
| POST | `/lab/kafka/ping` | Publish `lab.ping` |
| GET | `/lab/typesense` | Explore/search + stats |
| POST | `/lab/typesense/reindex` | Rebuild index dari Postgres |
| GET | `/lab/commerce/failure-matrix` | Matriks failure hidup |
| GET | `/lab/outbox/pending` | Unpublished outbox rows |
| POST | `/lab/outbox/pause` / `resume` | Chaos: hentikan / hidupkan relay loop |
| POST | `/lab/outbox/relay-once` | Paksa satu siklus publish |

---

## Notify — `internal/http/notify/`

Bukan CRUD; real-time fan-out.

| Symbol | Fungsi |
|--------|--------|
| `Hub` | Set koneksi WS; `Add`/`Remove`/`Broadcast`/`Count` |
| `Notifier.Run` | Loop `Fetch` Kafka (group notifier) → `Hub.Broadcast` → `Commit` |
| `Handler.products` | Upgrade HTTP → WebSocket `/ws/products` |
| `Handler.authenticate` | `apikey` + access JWT (`ParseAccessToken`); origin mengikuti `CORS_ORIGINS` |

Client: `ws://host/ws/products?token=<access>&apikey=<key>` → create/update product → event muncul di WS.

---

## Pola `mapErr` di setiap handler

Hampir semua feature punya `mapErr(c, err)`:

- `domain.Err*` → status HTTP + `FailCode`
- error validasi bind → 400
- sisanya → 500 (jangan bocorkan detail internal ke client)

Tiru file auth/product kalau menambah feature baru.

Lanjut → [07-worker.md](07-worker.md)
