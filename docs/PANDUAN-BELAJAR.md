# Panduan Belajar — Golang BE (API, Worker, Infra, Ops)

Dokumen praktek untuk setup **single-node LAN** (Ubuntu + k3s + Docker Compose).  
Default lab host: **`192.168.0.155`** — ganti jika IP server beda.

Baca sambil praktek. Jangan hanya scroll.

---

## Daftar isi

1. [URL & kredensial](#1-url--kredensial)
2. [Akun & dummy data](#2-akun--dummy-data)
3. [API baru untuk belajar (lab + wishlist)](#3-api-baru-untuk-belajar-lab--wishlist)
4. [Arsitektur singkat](#4-arsitektur-singkat)
5. [Setup dari nol](#5-setup-dari-nol)
6. [Menjalankan (day-to-day)](#6-menjalankan-day-to-day)
7. [Destroy / teardown](#7-destroy--teardown)
8. [Backup & restore](#8-backup--restore)
9. [Belajar API & Worker](#9-belajar-api--worker)
10. [Belajar tiap infra + use case](#10-belajar-tiap-infra--use-case)
11. [Observability](#11-observability)
12. [Troubleshooting](#12-troubleshooting)
13. [Latihan harian](#13-latihan-harian)
14. [Checklist keamanan lab](#14-checklist-keamanan-lab)

---

## 1. URL & kredensial

### App (k3s + Traefik)

| Layanan | URL |
|---------|-----|
| API | http://192.168.0.155/ |
| Health ready | http://192.168.0.155/health/ready |
| Swagger | http://192.168.0.155/docs |
| Metrics | http://192.168.0.155/metrics |

Header wajib di `/api/v1`: `apikey: dev-api-key`

Opsional di Mac `/etc/hosts`:

```
192.168.0.155  golang-be.local
```

### Infra (`~/projects/infra-db` + compose golang-be)

| Service | Endpoint | Kredensial |
|---------|----------|------------|
| PostgreSQL | `192.168.0.155:5432` | `admin` / `cQ5Fs5ciBO0ZOdk4` — DB app: `golang_be` |
| Redis | `192.168.0.155:6379` | tanpa password |
| MongoDB | `192.168.0.155:27017` | `admin` / `rLKK7Lo18d5M82gV` (`authSource=admin`) — DB audit: `golang_be_audit` |
| Kafka | `192.168.0.155:9092` | plaintext — topics: `products.events`, `products.events.dlq` |
| Typesense | http://192.168.0.155:8108 | `X-TYPESENSE-API-KEY: dev-typesense-key` |

Mongo URI (Secret k8s):

```
mongodb://admin:rLKK7Lo18d5M82gV@192.168.0.155:27017/?authSource=admin
```

### Observability (`make obs-up`)

| Service | URL | Login |
|---------|-----|-------|
| Grafana | http://192.168.0.155:3000 | `admin` / `admin` |
| Prometheus | http://192.168.0.155:9090 | — |
| Jaeger | http://192.168.0.155:16686 | butuh `OTEL_ENABLED=true` |

> Password di atas dari `~/projects/infra-db/docker-compose.yml`. Jangan commit `k8s/secret.yaml`.

---

## 2. Akun & dummy data

Seed lewat migration (jalan otomatis saat API boot):

| Email | Password | Role | Sumber |
|-------|----------|------|--------|
| `admin@golang-be.dev` | `admin123` | admin | `000002` |
| `seller@golang-be.dev` | `seller123` | user | `000003` |
| `buyer@golang-be.dev` | `user123` | user | `000003` |

**Dummy products (seller):** Kopi Susu Gula Aren, Matcha Latte, Croissant Butter, Americano, Brown Sugar Boba  
**Dummy products (admin):** Admin Merch Hoodie, Admin Sticker Pack  
**Dummy wishlist (buyer):** Kopi + Matcha  

Setelah deploy image baru / restart API, migration `000003`–`000004` ter-apply.  
Lalu **reindex Typesense** (index tidak ikut SQL seed):

```bash
TOKEN=$(curl -s http://192.168.0.155/api/v1/authentication/login \
  -H 'apikey: dev-api-key' -H 'Content-Type: application/json' \
  -d '{"email":"seller@golang-be.dev","password":"seller123"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["data"]["tokens"]["accessToken"])')

curl -s -X POST http://192.168.0.155/api/v1/lab/typesense/reindex \
  -H "apikey: dev-api-key" -H "Authorization: Bearer $TOKEN"
```

---

## 3. API baru untuk belajar (lab + wishlist)

Semua butuh `apikey` + JWT (kecuali health/docs).

### Wishlist — use case bisnis Postgres + Redis

| Method | Path | Infra | Konsep |
|--------|------|-------|--------|
| `POST` | `/api/v1/wishlists` | Postgres | Relasi many-to-many + unique constraint |
| `GET` | `/api/v1/wishlists` | Postgres | List milik user (ownership) |
| `GET` | `/api/v1/wishlists/count` | Redis cache-aside | Aggregate murah di-cache |
| `DELETE` | `/api/v1/wishlists/:id` | Postgres + invalidate Redis | Hapus + invalidate count |

Contoh (login sebagai **buyer**):

```bash
HOST=http://192.168.0.155
APIKEY='apikey: dev-api-key'
TOKEN=$(curl -s $HOST/api/v1/authentication/login \
  -H "$APIKEY" -H 'Content-Type: application/json' \
  -d '{"email":"buyer@golang-be.dev","password":"user123"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["data"]["tokens"]["accessToken"])')

curl -s $HOST/api/v1/wishlists -H "$APIKEY" -H "Authorization: Bearer $TOKEN"
curl -s $HOST/api/v1/wishlists/count -H "$APIKEY" -H "Authorization: Bearer $TOKEN"

# Tambah product lain (cek ID lewat lab/postgres/samples)
curl -s -X POST $HOST/api/v1/wishlists \
  -H "$APIKEY" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"productId":3,"note":"wishlist belajar"}'
```

### Lab — drill-down tiap infra

| Method | Path | Infra | Yang dipelajari |
|--------|------|-------|-----------------|
| `GET` | `/api/v1/lab/overview` | semua | Snapshot counts |
| `GET` | `/api/v1/lab/postgres/summary` | Postgres | Count users/products/wishlists |
| `GET` | `/api/v1/lab/postgres/samples` | Postgres | Sample join product↔owner |
| `GET` | `/api/v1/lab/redis/product/:id` | Redis | Cache hit/miss + TTL |
| `DELETE` | `/api/v1/lab/redis/product/:id` | Redis | Force invalidate |
| `GET` | `/api/v1/lab/mongo/events` | Mongo | Baca audit trail worker |
| `POST` | `/api/v1/lab/kafka/ping` | Kafka→Worker→Mongo | Publish event `lab.ping` |
| `GET` | `/api/v1/lab/typesense?q=kopi` | Typesense | Stats + search |
| `POST` | `/api/v1/lab/typesense/reindex` | PG→Typesense | Rebuild index dari source of truth |

Alur belajar recommended (15 menit):

```bash
# 1) overview
curl -s $HOST/api/v1/lab/overview -H "$APIKEY" -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

# 2) Redis: GET product → peek cache → invalidate → peek lagi
curl -s $HOST/api/v1/products/1 -H "$APIKEY" -H "Authorization: Bearer $TOKEN" >/dev/null
curl -s $HOST/api/v1/lab/redis/product/1 -H "$APIKEY" -H "Authorization: Bearer $TOKEN"
curl -s -X DELETE $HOST/api/v1/lab/redis/product/1 -H "$APIKEY" -H "Authorization: Bearer $TOKEN"

# 3) Kafka ping → worker audit → mongo events
curl -s -X POST $HOST/api/v1/lab/kafka/ping -H "$APIKEY" -H "Authorization: Bearer $TOKEN"
sleep 2
curl -s "$HOST/api/v1/lab/mongo/events?limit=5" -H "$APIKEY" -H "Authorization: Bearer $TOKEN"
```

### API produk (sudah ada) — pipeline penuh

`POST /products` → Postgres + Redis cache + Typesense index + Kafka → Worker Mongo + Notifier WebSocket.

---

## 4. Arsitektur singkat

```
Client
  → Traefik → API pods
                ├─ PostgreSQL (source of truth)
                ├─ Redis (cache + rate limit + wishlist count)
                ├─ Typesense (search index)
                └─ Kafka publish
                      ├─ worker → Mongo audit (+ DLQ)
                      └─ notifier (di API) → WebSocket
```

---

## 5. Setup dari nol

Jalankan di **server Ubuntu** kecuali langkah cek dari Mac.

### 5.1 Shared infra (Postgres / Redis / Mongo)

```bash
cd ~/projects/infra-db
docker compose up -d
docker ps | grep -E 'postgres-global|redis-global|mongo-global'
```

### 5.2 Clone app + env

```bash
git clone <repo> ~/projects/golang-be && cd ~/projects/golang-be
cp .env.example .env
# Sesuaikan DB_USER=admin, DB_PASSWORD, MONGO_URI ke infra-db
```

### 5.3 Kafka + Typesense

```bash
KAFKA_HOST_ADVERTISE=192.168.0.155 make infra-up
# Opsional:
make obs-up
```

### 5.4 k3s

```bash
curl -sfL https://get.k3s.io | sh -
sudo systemctl enable --now k3s
sudo k3s kubectl get nodes
```

### 5.5 Build & import image

```bash
docker build --target api    -t golang-be-api:local    .
docker build --target worker -t golang-be-worker:local .
docker save golang-be-api:local    -o /tmp/golang-be-api.tar
docker save golang-be-worker:local -o /tmp/golang-be-worker.tar
sudo k3s ctr images import /tmp/golang-be-api.tar
sudo k3s ctr images import /tmp/golang-be-worker.tar
```

### 5.6 ConfigMap + Secret

```bash
sed -i "s/HOST_IP/$(hostname -I | awk '{print $1}')/g" k8s/configmap.yaml
cp k8s/secret.example.yaml k8s/secret.yaml
# Isi secret: DB_*, TYPESENSE_API_KEY, JWT_SECRET, API_KEY,
# MONGO_URI=mongodb://admin:...@192.168.0.155:27017/?authSource=admin
```

### 5.7 Deploy

```bash
sudo k3s kubectl apply -f k8s/namespace.yaml
sudo k3s kubectl apply -f k8s/configmap.yaml -f k8s/secret.yaml
sudo k3s kubectl apply -f k8s/api-deployment.yaml -f k8s/api-service.yaml \
  -f k8s/api-hpa.yaml -f k8s/api-ingress.yaml -f k8s/worker-deployment.yaml
sudo k3s kubectl -n golang-be get pods -w
```

### 5.8 Verifikasi + seed search

```bash
curl http://192.168.0.155/health/ready
# login seller → POST /api/v1/lab/typesense/reindex (lihat section 2)
```

---

## 6. Menjalankan (day-to-day)

### Cek kesehatan cepat

```bash
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
sudo k3s kubectl -n golang-be get pods,svc,ingress
curl -s http://192.168.0.155/health/ready | python3 -m json.tool
```

### Restart app saja (tanpa rebuild)

```bash
sudo k3s kubectl -n golang-be rollout restart deploy/api deploy/worker
sudo k3s kubectl -n golang-be rollout status deploy/api
```

### Setelah ubah kode Go

```bash
cd ~/projects/golang-be
docker build --target api -t golang-be-api:local .
docker build --target worker -t golang-be-worker:local .
docker save golang-be-api:local -o /tmp/a.tar && sudo k3s ctr images import /tmp/a.tar
docker save golang-be-worker:local -o /tmp/w.tar && sudo k3s ctr images import /tmp/w.tar
sudo k3s kubectl -n golang-be rollout restart deploy/api deploy/worker
```

### Start / stop infra

```bash
# App deps
KAFKA_HOST_ADVERTISE=192.168.0.155 make infra-up
make infra-down          # hati-hati: stop kafka+typesense di compose ini

# Shared DB
cd ~/projects/infra-db && docker compose start|stop|restart

# Observability
make obs-up
make obs-down
```

### Logs

```bash
sudo k3s kubectl -n golang-be logs -f deploy/api
sudo k3s kubectl -n golang-be logs -f deploy/worker
docker logs -f golang-be-kafka-1
docker logs -f golang-be-typesense-1
```

### Shell ke DB

```bash
docker exec -it postgres-global psql -U admin -d golang_be
docker exec -it redis-global redis-cli
docker exec -it mongo-global mongosh \
  "mongodb://admin:rLKK7Lo18d5M82gV@127.0.0.1:27017/?authSource=admin"
```

---

## 7. Destroy / teardown

### Hapus workload k3s saja (infra Docker tetap)

```bash
sudo k3s kubectl delete namespace golang-be
# atau
sudo k3s kubectl -n golang-be delete deploy,svc,ingress,hpa,cm,secret --all
```

### Stop compose app (kafka/typesense/obs)

```bash
cd ~/projects/golang-be
make infra-down
# atau total termasuk volume kafka/typesense:
docker compose --profile observability down -v
```

### Stop shared infra (HATI-HATI: data hilang kalau `-v`)

```bash
cd ~/projects/infra-db
docker compose stop          # aman: data volume tetap
docker compose down          # container hilang, volume biasanya tetap
docker compose down -v       # ⚠️ hapus volume postgres/redis/mongo
```

### Uninstall k3s total

```bash
/usr/local/bin/k3s-uninstall.sh
```

---

## 8. Backup & restore

### PostgreSQL

```bash
# Backup
docker exec postgres-global pg_dump -U admin golang_be > ~/backup-golang_be-$(date +%F).sql

# Restore (DB harus ada)
docker exec -i postgres-global psql -U admin -d golang_be < ~/backup-golang_be-YYYY-MM-DD.sql
```

### MongoDB (audit)

```bash
docker exec mongo-global mongodump \
  --uri="mongodb://admin:rLKK7Lo18d5M82gV@127.0.0.1:27017/?authSource=admin" \
  --db=golang_be_audit --out=/tmp/mongodump

docker cp mongo-global:/tmp/mongodump ~/mongo-backup-$(date +%F)

# Restore
docker cp ~/mongo-backup-YYYY-MM-DD mongo-global:/tmp/mongodump
docker exec mongo-global mongorestore \
  --uri="mongodb://admin:rLKK7Lo18d5M82gV@127.0.0.1:27017/?authSource=admin" \
  /tmp/mongodump
```

### Redis (AOF sudah on di infra-db)

```bash
docker exec redis-global redis-cli BGSAVE
docker cp redis-global:/data/dump.rdb ~/redis-dump-$(date +%F).rdb
```

### Typesense

Tidak krusial di-backup: **rebuild dari Postgres** via `POST /api/v1/lab/typesense/reindex`.

### Kafka

Dev lab: topic bisa di-recreate. Data event “penting” sudah di Mongo audit + Postgres.  
Kalau perlu simpan volume: jangan `docker compose down -v` pada service kafka.

### Secret / ConfigMap k8s

```bash
sudo k3s kubectl -n golang-be get secret golang-be-secret -o yaml > ~/k8s-secret-backup.yaml
sudo k3s kubectl -n golang-be get configmap golang-be-config -o yaml > ~/k8s-cm-backup.yaml
chmod 600 ~/k8s-secret-backup.yaml
```

---

## 9. Belajar API & Worker

### API — urutan baca kode

1. `cmd/api/main.go` — fx wiring  
2. `internal/server/server.go` — middleware order  
3. `internal/auth/` — JWT  
4. `internal/product/` — CRUD + side effects  
5. `internal/wishlist/` — relasi + Redis count  
6. `internal/lab/` — eksplorasi infra  

### Worker

```bash
sudo k3s kubectl -n golang-be logs -f deploy/worker
```

1. `cmd/worker/main.go`  
2. `internal/audit/` — retry + DLQ  
3. `internal/platform/kafka/` — commit setelah sukses  

**Eksperimen:** `POST /lab/kafka/ping` → log `event audited type=lab.ping` → `GET /lab/mongo/events`.

### WebSocket

```bash
websocat "ws://192.168.0.155/ws/products?token=$TOKEN"
# di terminal lain: create product sebagai seller → event muncul
```

---

## 10. Belajar tiap infra + use case

### PostgreSQL

**Use case di project:** users, products, wishlists (source of truth).

```sql
\dt
SELECT email, role FROM users;
SELECT name, price, stock FROM products ORDER BY id;
SELECT w.id, u.email, p.name, w.note
FROM wishlists w
JOIN users u ON u.id = w.user_id
JOIN products p ON p.id = w.product_id;
```

API: `/lab/postgres/*`, `/wishlists`, `/products`.

### Redis

**Use case:** `product:{id}`, `products:user:{id}`, `wishlist:count:{userId}`, rate limit `rl:*`.

```text
KEYS *
TTL product:1
GET wishlist:count:3
```

API: GET product → `/lab/redis/product/:id` → DELETE invalidate → GET product lagi.

### MongoDB

**Use case:** immutable audit setiap event Kafka.

```javascript
use golang_be_audit
db.event_audit.find().sort({processedAt:-1}).limit(5).pretty()
```

API: `/lab/mongo/events` setelah create product atau `/lab/kafka/ping`.

### Kafka

```bash
docker exec -it golang-be-kafka-1 \
  /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 --topic products.events --from-beginning
```

Wajib: `KAFKA_HOST_ADVERTISE=<LAN_IP>` agar pod k3s tidak putus.

### Typesense

```bash
curl -s http://192.168.0.155:8108/collections \
  -H 'X-TYPESENSE-API-KEY: dev-typesense-key'
```

API: `/lab/typesense/reindex` lalu `/products/search?q=kopi` atau `/lab/typesense?q=kopi`.

---

## 11. Observability

```bash
make obs-up
# Grafana :3000 — lihat RED metrics dari /metrics
make obs-down   # hemat RAM
```

---

## 12. Troubleshooting

| Gejala | Arah perbaikan |
|--------|----------------|
| CrashLoopBackOff | `kubectl logs deploy/api` / `worker` |
| Panic `TypesenseAPIKey` + tag `url` | API key bukan URL — fix validasi config |
| Kafka timeout dari pod | Recreate kafka dengan `KAFKA_HOST_ADVERTISE=LAN_IP` |
| Worker Mongo Unauthorized | `MONGO_URI` di Secret harus ber-auth |
| Typesense Forbidden | Key `dev-typesense-key` harus sama di compose & secret |
| Search kosong padahal product ada | `POST /lab/typesense/reindex` |
| Image not found di k3s | Import `.tar` via `k3s ctr images import` |
| Wishlist 409 | Product sudah ada di wishlist (unique) |
| Migration tidak jalan | Migration hanya di API `OnStart` — pastikan API Running |
| Port bentrok | `ss -lntp \| grep -E '5432\|6379\|9092\|8108\|80'` |

Diagnostik cepat:

```bash
sudo k3s kubectl -n golang-be describe pod <pod>
sudo k3s kubectl -n golang-be get events --sort-by=.lastTimestamp | tail -30
docker inspect golang-be-kafka-1 --format '{{.State.Status}} {{.State.ExitCode}}'
```

### Rollback migration (dev only)

```bash
# Biasanya cukup fix SQL baru; kalau perlu down manual:
# pakai migrate CLI terhadap DB — atau restore dari backup section 8.
```

---

## 13. Latihan harian

| Hari | Fokus | Bukti selesai |
|------|--------|----------------|
| 1 | Setup + health | Pods Running + ready OK |
| 2 | Auth 3 akun | Login admin/seller/buyer |
| 3 | Postgres seed | `\dt` + query products/wishlists |
| 4 | Wishlist API | Add/list/count/delete |
| 5 | Redis cache | Peek hit → invalidate → miss |
| 6 | Kafka + Worker | `lab/kafka/ping` → mongo events |
| 7 | Typesense | Reindex + search `kopi` |
| 8 | WebSocket | Event create product masuk WS |
| 9 | Backup | Dump PG + restore ke DB test |
| 10 | Failure | Stop Typesense/Kafka, catat perilaku API |
| 11 | k3s ops | Rollout restart + baca events |
| 12 | Obs | 1 panel Grafana yang kamu jelaskan |

---

## 14. Checklist keamanan lab

- [ ] `k8s/secret.yaml` tidak pernah di-commit  
- [ ] Password infra-db hanya untuk LAN lab, bukan production public  
- [ ] Ganti `JWT_SECRET` / `API_KEY` kalau expose ke luar rumah  
- [ ] Jangan publish IP + password lab ke chat publik  
- [ ] Rotasi password SSH kalau pernah dishare  

---

## Referensi path

| Topik | Path |
|-------|------|
| Migrations / seed | `migrations/000003_*.sql`, `000004_*.sql` |
| Lab API | `internal/lab/` |
| Wishlist | `internal/wishlist/` |
| Manifests | `k8s/` |
| OpenAPI | `docs/openapi.yaml` |
| README utama | `README.md` |

Selamat belajar — tiap endpoint lab ada field `lesson` di response; baca itu juga.
