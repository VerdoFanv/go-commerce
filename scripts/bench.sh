#!/usr/bin/env bash
# Run the commerce-focused benchmark matrix and print a machine-readable summary.
#
# Usage (from repo root):
#   ./scripts/bench.sh                         # defaults: host network → 127.0.0.1:8080
#   BASE_URL=http://192.168.0.155 API_KEY=lab-api-key-change-in-prod ./scripts/bench.sh
#   BENCH_ONLY=smoke,oversell ./scripts/bench.sh
#
# Requires: Docker (grafana/k6 image). API must already be up (k3s or compose).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

K6_IMAGE="${K6_IMAGE:-grafana/k6:0.54.0}"
BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
API_KEY="${API_KEY:-dev-api-key}"
BENCH_ONLY="${BENCH_ONLY:-smoke,oversell,mixed,checkout,ratelimit}"
OUT_DIR="${OUT_DIR:-$ROOT/load/results}"
mkdir -p "$OUT_DIR"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
SUMMARY="$OUT_DIR/summary-$STAMP.json"

echo "==> probing $BASE_URL/health/ready"
code=$(curl -s -o /tmp/bench-ready.json -w "%{http_code}" "$BASE_URL/health/ready" || true)
if [[ "$code" != "200" ]]; then
  echo "API not ready (HTTP $code). Start lab stack first." >&2
  cat /tmp/bench-ready.json 2>/dev/null || true
  exit 1
fi
echo "ready OK"

run_k6() {
  local name="$1"
  local script="$2"
  shift 2
  local log="$OUT_DIR/${name}-$STAMP.log"
  echo ""
  echo "==> scenario: $name"
  # Linux lab box: --network host reaches Traefik / localhost ports.
  # Extra env vars (BENCH_*, OVERSELL_*) are forwarded as-is.
  set +e
  docker run --rm -i \
    --network host \
    -v "$ROOT/load:/scripts:ro" \
    -e BASE_URL="$BASE_URL" \
    -e API_KEY="$API_KEY" \
    -e BENCH_VUS="${BENCH_VUS:-}" \
    -e BENCH_DURATION="${BENCH_DURATION:-}" \
    -e BENCH_STOCK="${BENCH_STOCK:-}" \
    -e BENCH_THINK="${BENCH_THINK:-}" \
    -e OVERSELL_STOCK="${OVERSELL_STOCK:-}" \
    -e OVERSELL_VUS="${OVERSELL_VUS:-}" \
    "$@" \
    "$K6_IMAGE" run "/scripts/${script}" 2>&1 | tee "$log"
  local rc=${PIPESTATUS[0]}
  set -e
  echo "{\"scenario\":\"$name\",\"exit\":$rc,\"log\":\"$log\"}"
  return "$rc"
}

IFS=',' read -r -a SCENARIOS <<<"$BENCH_ONLY"
declare -a RESULTS=()
FAILED=0

for s in "${SCENARIOS[@]}"; do
  s="$(echo "$s" | tr -d ' ')"
  case "$s" in
    smoke)
      if run_k6 smoke smoke.js; then RESULTS+=("smoke:pass"); else RESULTS+=("smoke:fail"); FAILED=1; fi
      ;;
    oversell)
      if OVERSELL_STOCK="${OVERSELL_STOCK:-20}" OVERSELL_VUS="${OVERSELL_VUS:-60}" \
        run_k6 oversell oversell.js; then RESULTS+=("oversell:pass"); else RESULTS+=("oversell:fail"); FAILED=1; fi
      ;;
    mixed)
      if BENCH_VUS="${BENCH_VUS:-15}" BENCH_DURATION="${BENCH_DURATION:-90s}" \
        run_k6 mixed mixed.js; then RESULTS+=("mixed:pass"); else RESULTS+=("mixed:fail"); FAILED=1; fi
      ;;
    checkout)
      # Throughput path — expects elevated RATE_LIMIT_MAX on the API.
      if BENCH_VUS="${BENCH_VUS:-20}" BENCH_DURATION="${BENCH_DURATION:-2m}" BENCH_STOCK="${BENCH_STOCK:-5000}" \
        run_k6 checkout checkout.js; then RESULTS+=("checkout:pass"); else RESULTS+=("checkout:fail"); FAILED=1; fi
      ;;
    ratelimit)
      if run_k6 ratelimit ratelimit.js; then RESULTS+=("ratelimit:pass"); else RESULTS+=("ratelimit:fail"); FAILED=1; fi
      ;;
    *)
      echo "unknown scenario: $s" >&2
      FAILED=1
      ;;
  esac
done

{
  echo "{"
  echo "  \"timestamp\": \"$STAMP\","
  echo "  \"base_url\": \"$BASE_URL\","
  echo "  \"results\": ["
  for i in "${!RESULTS[@]}"; do
    name="${RESULTS[$i]%%:*}"; status="${RESULTS[$i]#*:}"
    comma=","; [[ $i -eq $((${#RESULTS[@]} - 1)) ]] && comma=""
    echo "    {\"scenario\": \"$name\", \"status\": \"$status\"}$comma"
  done
  echo "  ]"
  echo "}"
} | tee "$SUMMARY"

echo ""
echo "Summary written to $SUMMARY"
exit "$FAILED"
