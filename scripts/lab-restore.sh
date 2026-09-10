#!/usr/bin/env bash
# Bring the single-node lab back to a normal, healthy state after chaos / bench.
#
# Safe to run anytime (idempotent). Typical use:
#   ./scripts/lab-restore.sh
#   HOST=http://192.168.0.155 API_KEY=lab-api-key-change-in-prod ./scripts/lab-restore.sh
#
# Restores:
#   - Docker Compose data plane (kafka, typesense) in this repo
#   - infra-db (postgres, redis, mongo) if ~/projects/infra-db exists
#   - Outbox relay resume + Redis pause key cleared
#   - RATE_LIMIT_MAX back to ConfigMap default (100) unless KEEP_RATE_LIMIT=1
#   - Waits until /health/ready reports message=ready (not degraded)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

HOST="${HOST:-http://127.0.0.1:8080}"
API_KEY="${API_KEY:-dev-api-key}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@golang-be.dev}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123}"
HOST_IP="${HOST_IP:-$(hostname -I 2>/dev/null | awk '{print $1}')}"
KEEP_RATE_LIMIT="${KEEP_RATE_LIMIT:-0}"
INFRA_DB="${INFRA_DB:-$HOME/projects/infra-db}"

SUDO() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    "$@"
  fi
}

echo "==> data plane: kafka + typesense"
if [[ -n "${HOST_IP:-}" ]]; then
  KAFKA_HOST_ADVERTISE="$HOST_IP" docker compose up -d kafka typesense >/dev/null
else
  docker compose up -d kafka typesense >/dev/null
fi

if [[ -d "$INFRA_DB" ]]; then
  echo "==> infra-db: postgres + redis + mongo"
  (cd "$INFRA_DB" && docker compose up -d >/dev/null)
else
  echo "==> skip infra-db (not found at $INFRA_DB)"
fi

echo "==> wait ports"
for p in 5432 6379 27017 9092 8108; do
  for i in $(seq 1 40); do
    if (echo >/dev/tcp/127.0.0.1/"$p") >/dev/null 2>&1; then
      echo "  port $p up"
      break
    fi
    sleep 1
  done
done

# Clear shared outbox pause flag even before API is ready.
if command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' | grep -qx redis-global; then
  docker exec redis-global redis-cli DEL outbox:relay:paused >/dev/null 2>&1 || true
  echo "==> cleared Redis key outbox:relay:paused"
fi

# Restore rate limit to repo default (bench/chaos often bump it).
if [[ "$KEEP_RATE_LIMIT" != "1" ]] && command -v k3s >/dev/null 2>&1; then
  if SUDO k3s kubectl -n golang-be get cm golang-be-config >/dev/null 2>&1; then
    echo "==> RATE_LIMIT_MAX → 100 (set KEEP_RATE_LIMIT=1 to skip)"
    SUDO k3s kubectl -n golang-be patch cm golang-be-config --type merge \
      -p '{"data":{"RATE_LIMIT_MAX":"100","RATE_LIMIT_WINDOW":"1m"}}' >/dev/null
    # Restart api so limiter reloads env from ConfigMap
    SUDO k3s kubectl -n golang-be rollout restart deploy/api >/dev/null 2>&1 || true
    SUDO k3s kubectl -n golang-be rollout status deploy/api --timeout=180s >/dev/null 2>&1 || true
  fi
fi

echo "==> wait /health/ready = ready (not degraded)"
for i in $(seq 1 60); do
  body=$(curl -s "$HOST/health/ready" || true)
  msg=$(echo "$body" | python3 -c "import sys,json; print(json.load(sys.stdin).get('message',''))" 2>/dev/null || true)
  if [[ "$msg" == "ready" ]]; then
    echo "$body" | python3 -m json.tool 2>/dev/null || echo "$body"
    break
  fi
  if [[ "$i" -eq 60 ]]; then
    echo "WARN: ready not fully healthy yet:" >&2
    echo "$body" >&2
  fi
  sleep 2
done

# Resume outbox via lab API (best-effort).
echo "==> outbox resume (lab API)"
token=$(curl -s -X POST "$HOST/api/v1/authentication/login" \
  -H "apikey: $API_KEY" -H "Content-Type: application/json" \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['tokens']['accessToken'])" 2>/dev/null || true)
if [[ -n "${token:-}" ]]; then
  curl -s -X POST "$HOST/api/v1/lab/outbox/resume" \
    -H "apikey: $API_KEY" -H "Authorization: Bearer $token" | python3 -m json.tool 2>/dev/null || true
  curl -s -X POST "$HOST/api/v1/lab/outbox/relay-once" \
    -H "apikey: $API_KEY" -H "Authorization: Bearer $token" >/dev/null || true
else
  echo "  skip lab resume (admin login failed — Redis pause key already cleared)"
fi

echo "==> final probe"
curl -s "$HOST/health/ready" | python3 -m json.tool 2>/dev/null || curl -s "$HOST/health/ready"
echo ""
echo "Lab restore done. If Typesense was empty after chaos, run POST /lab/typesense/reindex (admin)."
