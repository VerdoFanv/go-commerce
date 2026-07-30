# Testing Guide

Struktur ini setara folder `test` + Jest di Express, disesuaikan konvensi Go enterprise.

## Mapping dari Jest

| Jest (Express) | Go (project ini) |
|---|---|
| `test/` / `__tests__/` | `test/` |
| `jest.setup.js` helpers | `test/testutil/` |
| `jest.mock(...)` | `test/mocks/` |
| unit `*.test.ts` | `test/unit/<feature>/` |
| `supertest` API tests | `test/integration/` + `httptest` |
| `npm test` | `make test` |

## Layout

```
test/
  testutil/          # shared helpers (config, JWT, HTTP client)
  mocks/             # in-memory repositories
  unit/
    auth/
    product/
    middleware/
    response/
  integration/       # HTTP API tests (handler + service + mock repo)
```

## Commands

```bash
make test              # semua test
make test-unit         # unit only
make test-integration  # API/integration only
make test-race         # dengan race detector
make test-cover        # coverage HTML → coverage.html
```

## Aturan

1. **Jangan** taruh `*_test.go` di samping source (`internal/...`) kecuali white-box test yang benar-benar butuh unexported symbol.
2. Unit test = black-box (`package xxx_test`), mock lewat interface.
3. Integration test = wire Fiber app seperti production, tapi repo in-memory (tanpa Postgres/Redis/MQ).
4. Assertion: `testify/require` (fail-fast).
5. Config test: selalu `testutil.Config()` (`BcryptCost=MinCost`).
6. Feature baru → tambah `test/unit/<feature>/` + endpoint baru di `test/integration/` bila ada HTTP surface.
