# Chaos — LitmusChaos

Infra failure evidence for the k3s lab. **Load/capacity** stays in Grafana k6 (`load/` + `make bench`).

| Tool | Proves |
|------|--------|
| **Litmus** (`make chaos`) | API/worker survive **pod delete**; readiness probe stays green |
| **Outbox SLI** (`make chaos-outbox`) | Dual-write: order 201 while relay paused, then drain |
| **k6** (`make bench`) | Oversell, checkout p95, rate limit — not chaos |

## Layout

```
chaos/
  rbac.yaml                 # litmus-admin SA/Role in golang-be
  experiments/pod-delete.yaml
  engines/api-pod-delete.yaml
  engines/worker-pod-delete.yaml
scripts/litmus-install.sh   # operator + experiment (once)
scripts/litmus-run.sh       # apply engines, wait ChaosResult
```

## One-time install (lab box with k3s)

```bash
# on the server that runs k3s
cd ~/projects/golang-be
SUDO_PASS='…' ./scripts/litmus-install.sh   # or run as root
```

Installs the Litmus **operator only** (no ChaosCenter UI — too heavy for a single APU node).

## Run

```bash
HOST=http://192.168.0.155 API_KEY=lab-api-key-change-in-prod make chaos
make chaos-api      # only API pod-delete
make chaos-worker   # only worker pod-delete

# Optional: also prove outbox dual-write
APP_SLI=1 HOST=http://192.168.0.155 API_KEY=lab-api-key-change-in-prod ./scripts/litmus-run.sh
# or:
make chaos-outbox
```

Engines clean themselves up on EXIT (`CLEANUP=0` to keep CRs for debugging).

## Pass criteria

| Engine | Fault | Probe | Pass |
|--------|-------|-------|------|
| `api-pod-delete` | Delete ~50% of `app=api` pods | Continuous HTTP `GET http://api.golang-be.svc/health/ready` → 200 | ChaosResult verdict `Pass` + deploy Ready |
| `worker-pod-delete` | Delete worker pod | Continuous HTTP ready on API Service | Verdict `Pass` + worker recreated |

Afterward: `./scripts/lab-restore.sh` if you also stopped Compose deps manually.

## Notes

- Targets labels already on Deployments: `app=api`, `app=worker`.
- Experiment image pinned: `litmuschaos/go-runner:3.16.0`.
- `annotationCheck: false` — no need to annotate pods with `litmuschaos.io/chaos`.
