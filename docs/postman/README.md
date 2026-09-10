# Postman — Golang BE

## Import (re-import after updates)

1. Import **both**:
   - `Golang-BE.postman_collection.json`
   - `Golang-BE.lab.postman_environment.json` **or** `Golang-BE.local.postman_environment.json`
2. Select the environment (top-right).
3. Run **1. Auth → Login (buyer)** then **Login (admin)**.
4. Confirm env vars `accessToken` / `refreshToken` / `adminToken` are filled.

## Fixes baked into this collection

| Issue | Fix |
|-------|-----|
| Tokens not visible in env | Scripts call `pm.environment.set` (env wins over collection) |
| Broken URLs with Lab env | Requests use string `"{{baseUrl}}/path"` (not `host: ["{{baseUrl}}"]`) |
| Wishlist/order 400 | `productId` is a **JSON number** `{{productId}}`, not `"{{productId}}"` |
| Missing `wsUrl` | Defined on collection + both envs |
| Cancel before Fulfill | Fulfill runs before Cancel in folder 4 |

## Happy path

1. Health → Ready  
2. Auth → Login (buyer) + Login (admin)  
3. Products → Create product  
4. Wishlists → Add  
5. Orders → Create → Get → Pay → Fulfill (admin)  
6. Lab → Failure matrix / outbox (admin)

## Auth response shapes

| Endpoint | `data` |
|----------|--------|
| register / login | `{ user, tokens: { accessToken, refreshToken } }` |
| refresh-token | `{ accessToken, refreshToken }` |

Spec: [`../openapi.yaml`](../openapi.yaml) · Chaos: [`../FAILURE-RUNBOOK.md`](../FAILURE-RUNBOOK.md).
