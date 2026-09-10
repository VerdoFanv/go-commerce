# Postman — Golang BE

## Import

1. Postman → **Import**:
   - `Golang-BE.postman_collection.json`
   - `Golang-BE.lab.postman_environment.json` **or** `Golang-BE.local.postman_environment.json`
2. Select the environment in the top-right (Lab / Local).
3. Run **1. Auth → Login (buyer/seller)**, then **Login (admin)**.

Login / refresh / register **Tests** scripts save tokens to the **active Environment**
(`accessToken`, `refreshToken`, `adminToken`) and to collection variables.
Environment wins variable resolution — that is why tokens must be written there
(empty env values used to shadow collection-only saves).

After login, open the environment editor and confirm `accessToken` is filled.
Re-import the collection if you still have an older copy without the fix.

## Happy path (5 minutes)

1. Health → Ready  
2. Auth → Login (buyer) + Login (admin)  
3. Products → Create product  
4. Wishlists → Add  
5. Orders → Create order → Get order  
6. Lab → Failure matrix + Outbox pause/pending/resume (admin)

## Notes

- All `/api/v1/*` need header `apikey`.
- `POST /orders` requires `Idempotency-Key` (collection uses `{{$guid}}`).
- Lab folder needs `APP_ENV!=production` + admin role.
- Spec: [`../openapi.yaml`](../openapi.yaml) · Chaos: [`../FAILURE-RUNBOOK.md`](../FAILURE-RUNBOOK.md).

### Auth response shapes (match OpenAPI)

| Endpoint | `data` shape |
|----------|----------------|
| register / login | `{ user, tokens: { accessToken, refreshToken } }` |
| refresh-token | `{ accessToken, refreshToken }` (tokens at top of `data`) |
