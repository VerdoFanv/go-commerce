# Benchmark & capacity evidence

This document exists because README feature lists are not proof. Here we define
**what we measure**, **how to run it**, and **how to read the numbers** on the
single-node lab (Compose data plane + k3s app, or full Compose).

Unit/integration tests (`test/`) prove **logic with mocks**. They do **not**
prove capacity, oversell under real Postgres locks, or outbox lag. Load scripts
here close that gap.

---

## What “good enough” means for this portfolio box

We are not claiming AWS-scale. We claim: on **this** hardware + stack, under a
documented matrix, the system:

1. Completes checkout (order + stock hold + outbox) within a latency budget.
2. **Never oversells** under parallel contention (`stock >= 0`, `created <= stock`).
3. Keeps error rate low under mixed browse/buy traffic.
4. Surfaces rate limiting as an intentional 429 (not 5xx).
5. Exposes outbox lag (`golangbe_outbox_pending`) so you can see dual-write safety under write load.

---

## Scenario matrix

| ID | Script | Question it answers | Pass criteria (defaults) |
|----|--------|---------------------|--------------------------|
| A | `load/smoke.js` | Does health → register → product → **order** work? | 0 HTTP failures; stock ≥ 0 |
| B | `load/oversell.js` | Can N≫S buyers race one SKU without negative stock? | `created ≤ S`, `oversell_other=0`, teardown stock ≥ 0 |
| C | `load/mixed.js` | 80% catalog / 20% buy — storefront-ish mix | browse p95 &lt; 300ms, buy p95 &lt; 600ms, fail &lt; 5% |
| D | `load/checkout.js` | Sustained `POST /orders` throughput + p95 | order p95 &lt; 500ms, p99 &lt; 1.5s, fail &lt; 5% |
| E | `load/ratelimit.js` | Does Redis sliding window actually trip? | ≥1 × 429, 0 × 5xx |

Legacy: `scripts/load-orders.sh` — bash oversell with **hard assert** (same SLI as B).

---

## How to run

### Prereqs

- API ready: `curl -s $BASE_URL/health/ready` → 200
- Docker available (k6 runs in `grafana/k6:0.54.0`)
- For **D (checkout)**: temporarily raise API `RATE_LIMIT_MAX` (e.g. `10000`) or you measure the limiter, not Postgres. Restore after.
- For **E (ratelimit)**: keep a **low** `RATE_LIMIT_MAX` (e.g. `30` / `1m`) so 429s appear quickly.

### One command

```bash
# Lab (k3s ingress) example:
BASE_URL=http://192.168.0.155 \
API_KEY=lab-api-key-change-in-prod \
./scripts/bench.sh

# Subset:
BENCH_ONLY=smoke,oversell ./scripts/bench.sh

# Compose API on localhost:
BASE_URL=http://127.0.0.1:8080 API_KEY=dev-api-key ./scripts/bench.sh
```

Makefile shortcuts:

```bash
make bench              # full matrix via scripts/bench.sh
make bench-smoke
make bench-oversell
make load-smoke         # alias → smoke (commerce path)
```

Logs + JSON summary land in `load/results/` (gitignored).

### Observability during a run

```bash
curl -s $BASE_URL/metrics | grep -E 'golangbe_http_request_duration|golangbe_outbox_pending|golangbe_events_'
# or Grafana (make obs-up): RPS, p95 by route, publish/consume, DLQ
```

After checkout load stops, watch `golangbe_outbox_pending` fall toward 0 (relay draining).

---

## Interpreting results (senior checklist)

| Observation | Likely meaning |
|-------------|----------------|
| Oversell fail (stock &lt; 0 or created &gt; S) | **P0** — hold TX / WHERE stock≥qty broken |
| Checkout p95 blows up, DB CPU pegged | Connection pool / missing index / lock contention on popular SKU |
| Many 429, low p95 | You hit rate limit — raise `RATE_LIMIT_MAX` for throughput runs |
| Outbox pending climbs and never drains | Relay paused, Kafka unreachable, or publish errors |
| DLQ rising under “happy” load | Worker bugs / poison payloads — not a capacity win |
| Smoke pass, checkout fail | Logic OK under mock-ish load; hot path needs work |

**Do not** paste vanity RPS into the README without: hardware note, `RATE_LIMIT_MAX`, VU count, duration, and pass/fail of oversell.

---

## Lab result log

Fill a row every time you run on a real box (honest numbers &gt; marketing).

| Date (UTC) | Host / HW | Stack | RATE_LIMIT_MAX | smoke | oversell | mixed | checkout (p95 / approx RPS) | Notes |
|------------|-----------|-------|----------------|-------|----------|-------|-----------------------------|-------|
| _(run `./scripts/bench.sh` and paste)_ | | k3s+compose | | | | | | |

Example row format after a run:

```text
2026-09-10 | 192.168.0.155 (lab) | k3s api×2 + worker×1, kafka/typesense compose | 10000 | pass | pass (S=20,VUs=60) | pass | p95=..ms ~N orders/s | outbox drained
```

---

## Relation to `test/`

| Layer | Proves | Does not prove |
|-------|--------|----------------|
| `test/unit` | Business rules, FSM, mocks | Real DB races |
| `test/integration` | HTTP wiring with in-memory repos | Kafka, Redis RL, stock locks |
| `load/*` + `scripts/bench.sh` | Capacity + contention SLIs on a live stack | Multi-region / multi-AZ |

All three are required. Shipping only unit tests + a long README is how portfolios lie by omission.
