#!/usr/bin/env bash
# Validate k6 scenario syntax (no API required).
# Prefers local `k6` binary; falls back to grafana/k6 Docker image.
#
# Usage:
#   ./scripts/k6-validate.sh
#   ./scripts/k6-validate.sh smoke.js oversell.js
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

K6_IMAGE="${K6_IMAGE:-grafana/k6:0.54.0}"

SCRIPTS=()
if [[ "$#" -gt 0 ]]; then
  SCRIPTS=("$@")
else
  for f in "$ROOT"/load/*.js; do
    base="$(basename "$f")"
    [[ "$base" == "helpers.js" ]] && continue
    SCRIPTS+=("$base")
  done
fi

if [[ "${#SCRIPTS[@]}" -eq 0 ]]; then
  echo "no k6 scripts found under load/" >&2
  exit 1
fi

run_inspect() {
  local script="$1"
  if command -v k6 >/dev/null 2>&1; then
    k6 inspect --execution-requirements "$ROOT/load/$script" >/dev/null
    return
  fi
  if command -v docker >/dev/null 2>&1; then
    docker run --rm -i \
      -v "$ROOT/load:/scripts:ro" \
      -e BASE_URL=http://127.0.0.1:8080 \
      -e API_KEY=dev-api-key \
      "$K6_IMAGE" inspect --execution-requirements "/scripts/$script" >/dev/null
    return
  fi
  echo "need local k6 binary or docker (grafana/k6)" >&2
  return 127
}

if command -v k6 >/dev/null 2>&1; then
  echo "==> using local k6 ($(k6 version 2>/dev/null | head -1))"
elif command -v docker >/dev/null 2>&1; then
  echo "==> using docker image $K6_IMAGE"
  docker pull -q "$K6_IMAGE" >/dev/null
else
  echo "need local k6 binary or docker" >&2
  exit 1
fi

failed=0
for s in "${SCRIPTS[@]}"; do
  echo "==> inspect load/$s"
  errf="$(mktemp)"
  if run_inspect "$s" 2>"$errf"; then
    echo "  OK"
  else
    echo "  FAIL"
    cat "$errf" >&2 || true
    failed=1
  fi
  rm -f "$errf"
done

[[ "$failed" -eq 0 ]]
echo "k6 validate OK (${#SCRIPTS[@]} scripts)"
