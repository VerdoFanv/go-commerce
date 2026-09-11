#!/usr/bin/env bash
# Run LitmusChaos engines against golang-be (api + worker pod-delete).
#
# Prereq: ./scripts/litmus-install.sh once on the lab box.
#
# Usage:
#   ./scripts/litmus-run.sh
#   CHAOS_ONLY=api ./scripts/litmus-run.sh
#   CHAOS_ONLY=worker HOST=http://192.168.0.155 ./scripts/litmus-run.sh
#
# Optional app-level outbox SLI (HTTP, not Litmus): APP_SLI=1
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NS_APP="${NS_APP:-golang-be}"
HOST="${HOST:-http://127.0.0.1:8080}"
API_KEY="${API_KEY:-dev-api-key}"
CHAOS_ONLY="${CHAOS_ONLY:-api,worker}"
APP_SLI="${APP_SLI:-0}"
WAIT_SEC="${WAIT_SEC:-240}"
CLEANUP="${CLEANUP:-1}"

SUDO() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  elif [[ -n "${SUDO_PASS:-}" ]]; then
    printf '%s\n' "$SUDO_PASS" | sudo -S -p '' "$@"
  elif sudo -n true 2>/dev/null; then
    sudo -n "$@"
  else
    echo "need sudo for k3s kubectl (set SUDO_PASS or run as root)" >&2
    return 1
  fi
}

K() { SUDO k3s kubectl "$@"; }

pass=0
fail=0
ok() { echo "  PASS: $*"; pass=$((pass + 1)); }
bad() { echo "  FAIL: $*"; fail=$((fail + 1)); }

cleanup_engines() {
  [[ "$CLEANUP" == "1" ]] || return 0
  echo ""
  echo "==> cleanup ChaosEngines"
  K -n "$NS_APP" delete chaosengine api-pod-delete worker-pod-delete --ignore-not-found >/dev/null 2>&1 || true
}
trap cleanup_engines EXIT

echo "==> probe API $HOST/health/ready"
code=$(curl -s -o /tmp/chaos-ready.json -w "%{http_code}" "$HOST/health/ready" || true)
if [[ "$code" != "200" ]]; then
  echo "API not ready (HTTP $code). Start lab stack first." >&2
  cat /tmp/chaos-ready.json 2>/dev/null || true
  exit 1
fi
ok "health/ready HTTP 200"

echo "==> check Litmus CRDs + experiment"
if ! K get crd chaosengines.litmuschaos.io >/dev/null 2>&1; then
  echo "Litmus not installed. Run: ./scripts/litmus-install.sh" >&2
  exit 1
fi
if ! K -n "$NS_APP" get chaosexperiment pod-delete >/dev/null 2>&1; then
  echo "ChaosExperiment missing — applying install pieces" >&2
  K apply -f "$ROOT/chaos/rbac.yaml"
  K apply -f "$ROOT/chaos/experiments/pod-delete.yaml"
fi
ok "Litmus CRDs + pod-delete experiment"

if [[ "$APP_SLI" == "1" ]]; then
  echo "==> app SLI (outbox pause/resume via legacy script)"
  if HOST="$HOST" API_KEY="$API_KEY" "$ROOT/scripts/chaos-outbox.sh"; then
    ok "outbox dual-write SLI"
  else
    bad "outbox dual-write SLI"
  fi
fi

run_engine() {
  local name="$1"
  local file="$2"
  echo ""
  echo "==> ChaosEngine: $name"
  K -n "$NS_APP" delete chaosengine "$name" --ignore-not-found >/dev/null 2>&1 || true
  # Fresh apply: engines are one-shot; delete any prior result so we do not read stale verdict.
  K -n "$NS_APP" delete chaosresult "${name}-pod-delete" --ignore-not-found >/dev/null 2>&1 || true
  K apply -f "$file"

  local deadline=$((SECONDS + WAIT_SEC))
  local verdict=""
  local phase=""
  while true; do
    # Result name is typically <engine>-<experiment>
    verdict=$(K -n "$NS_APP" get chaosresult "${name}-pod-delete" -o jsonpath='{.status.experimentStatus.verdict}' 2>/dev/null || true)
    phase=$(K -n "$NS_APP" get chaosengine "$name" -o jsonpath='{.status.engineStatus}' 2>/dev/null || true)
    if [[ "$verdict" == "Pass" || "$verdict" == "Fail" || "$verdict" == "Stopped" ]]; then
      break
    fi
    if [[ "$phase" == "completed" || "$phase" == "stopped" ]]; then
      # Give result a moment to land.
      sleep 2
      verdict=$(K -n "$NS_APP" get chaosresult "${name}-pod-delete" -o jsonpath='{.status.experimentStatus.verdict}' 2>/dev/null || true)
      break
    fi
    if (( SECONDS >= deadline )); then
      echo "timeout waiting for $name (phase=$phase verdict=$verdict)" >&2
      K -n "$NS_APP" get chaosengine,chaosresult,pods -o wide 2>&1 || true
      return 1
    fi
    sleep 3
  done

  echo "  engine=$name phase=${phase:-?} verdict=${verdict:-?}"
  if [[ "$verdict" == "Pass" ]]; then
    ok "$name verdict=Pass"
    return 0
  fi
  bad "$name verdict=${verdict:-unknown}"
  K -n "$NS_APP" get chaosresult "${name}-pod-delete" -o yaml 2>&1 | tail -80 || true
  return 1
}

IFS=',' read -r -a ENGINES <<<"$CHAOS_ONLY"
for e in "${ENGINES[@]}"; do
  e="$(echo "$e" | tr -d ' ')"
  case "$e" in
    api)
      run_engine api-pod-delete "$ROOT/chaos/engines/api-pod-delete.yaml" || true
      ;;
    worker)
      run_engine worker-pod-delete "$ROOT/chaos/engines/worker-pod-delete.yaml" || true
      ;;
    *)
      echo "unknown engine: $e (want api|worker)" >&2
      fail=$((fail + 1))
      ;;
  esac
done

echo ""
echo "==> post-chaos rollout status"
K -n "$NS_APP" rollout status deploy/api --timeout=120s >/dev/null && ok "api rollout Ready" || bad "api rollout"
K -n "$NS_APP" rollout status deploy/worker --timeout=120s >/dev/null && ok "worker rollout Ready" || bad "worker rollout"

code=$(curl -s -o /dev/null -w "%{http_code}" "$HOST/health/ready" || true)
[[ "$code" == "200" ]] && ok "health/ready after chaos" || bad "health/ready after chaos HTTP $code"

echo ""
echo "Result: $pass passed, $fail failed"
echo "Restore helpers: ./scripts/lab-restore.sh"
[[ "$fail" -eq 0 ]]
