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
| load / k6 | `load/` + `scripts/bench.sh` |

## Layout

```
test/
  testutil/          # shared helpers (config, JWT, HTTP client)
  mocks/             # in-memory repositories
  unit/
    auth|product|order|wishlist|middleware|domain|payment|inventory|audit/
  integration/       # HTTP API tests (handler + service + mock repo)

load/                # k6 scenarios against a LIVE stack (capacity SLIs)
  helpers.js
  smoke.js | oversell.js | mixed.js | checkout.js | ratelimit.js
scripts/bench.sh     # matrix runner → load/results/
docs/BENCHMARK.md    # what we measure + how to read numbers
```

## Commands

```bash
make test              # unit + integration (mocks — no Postgres/Kafka)
make test-unit
make test-integration
make test-race
make test-cover

# Live stack required:
make bench             # full commerce matrix
make bench-smoke
make bench-oversell
```

## Aturan

1. **Jangan** taruh `*_test.go` di samping source (`internal/...`) kecuali white-box yang wajib unexported.
2. Unit = black-box (`package xxx_test`), mock lewat interface.
3. Integration = Gin + in-memory repo (**bukan** bukti kapasitas / oversell).
4. Assertion: `testify/require`.
5. Config test: `testutil.Config()` (`BcryptCost=MinCost`).
6. Feature baru → `test/unit/<feature>/` + integration bila ada HTTP surface.
7. Klaim “aman concurrent / tahan load” → wajib ada skenario di `load/` + baris di `docs/BENCHMARK.md`.

## Honest split

| Layer | Proves | Does not prove |
|-------|--------|----------------|
| unit/integration | Correctness of rules & wiring | Real row locks, Kafka lag, RL |
| `load/` bench | Oversell, checkout p95, 429 SLI on lab box | Multi-region prod SLA |
