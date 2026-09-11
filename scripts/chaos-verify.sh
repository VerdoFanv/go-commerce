#!/usr/bin/env bash
# Deprecated wrapper: infra chaos moved to Litmus (make chaos).
# This script now runs Litmus engines; set LEGACY_OUTBOX=1 for old HTTP outbox SLI only.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if [[ "${LEGACY_OUTBOX:-0}" == "1" ]]; then
  exec "$ROOT/scripts/chaos-outbox.sh" "$@"
fi
echo "NOTE: chaos-verify → Litmus (scripts/litmus-run.sh). Use LEGACY_OUTBOX=1 for outbox-only SLI."
exec "$ROOT/scripts/litmus-run.sh" "$@"
