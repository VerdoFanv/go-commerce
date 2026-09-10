# Postman — Golang BE

## Import

1. Postman → **Import** → pilih:
   - `Golang-BE.postman_collection.json`
   - `Golang-BE.lab.postman_environment.json` **atau** `Golang-BE.local.postman_environment.json`
2. Pilih environment di kanan atas.
3. Jalankan **1. Auth → Login (buyer/seller)** lalu **Login (admin)** — script menyimpan token.

## Happy path (5 menit)

1. Health → Ready  
2. Auth → Login (buyer) + Login (admin)  
3. Products → Create product  
4. Wishlists → Add  
5. Orders → Create order → Get order  
6. Lab → Failure matrix + Outbox pause/pending/resume (admin)

## Catatan

- Semua `/api/v1/*` butuh header `apikey`.
- `POST /orders` wajib `Idempotency-Key` (collection mengisi `{{$guid}}`).
- Folder **Lab** butuh `APP_ENV!=production` + role admin.
- Spec lengkap: [`../openapi.yaml`](../openapi.yaml) (juga di `/docs` non-prod).
- Chaos / recovery: [`../FAILURE-RUNBOOK.md`](../FAILURE-RUNBOOK.md).
