#!/usr/bin/env bash
# App-level outbox dual-write SLI (HTTP lab endpoints) — not Litmus.
# Infra chaos (pod kill) lives in Litmus: ./scripts/litmus-run.sh / make chaos
#
# Usage:
#   HOST=http://192.168.0.155 API_KEY=lab-api-key-change-in-prod ./scripts/chaos-outbox.sh
#   APP_SLI=1 ./scripts/litmus-run.sh   # runs this after Litmus install check
#
# Always leaves the lab usable: on EXIT it resumes outbox + clears the Redis
# pause key. For full container restore after manual stop/start chaos, run:
#   ./scripts/lab-restore.sh
#
# Optional: ADMIN_EMAIL ADMIN_PASSWORD BUYER_EMAIL BUYER_PASSWORD PRODUCT_ID
#           SKIP_RESTORE=1  — disable EXIT cleanup (debug only)
set -euo pipefail

HOST="${HOST:-http://127.0.0.1:8080}"
API_KEY="${API_KEY:-dev-api-key}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@golang-be.dev}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123}"
BUYER_EMAIL="${BUYER_EMAIL:-buyer@golang-be.dev}"
BUYER_PASSWORD="${BUYER_PASSWORD:-user123}"
PRODUCT_ID="${PRODUCT_ID:-}"
SKIP_RESTORE="${SKIP_RESTORE:-0}"

HDR=(-H "apikey: ${API_KEY}" -H "Content-Type: application/json")
pass=0
fail=0
ADMIN_TOKEN=""

ok() { echo "  PASS: $*"; pass=$((pass + 1)); }
bad() { echo "  FAIL: $*"; fail=$((fail + 1)); }

login() {
  local email="$1" passwd="$2"
  local raw
  raw=$(curl -s -X POST "$HOST/api/v1/authentication/login" "${HDR[@]}" \
    -d "{\"email\":\"$email\",\"password\":\"$passwd\"}")
  echo "$raw" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['data']['tokens']['accessToken'])" 2>/dev/null \
    || { echo "login failed for $email: $raw" >&2; return 1; }
}

# Never leave outbox paused if this script crashes mid-chaos.
cleanup_outbox() {
  [[ "$SKIP_RESTORE" == "1" ]] && return 0
  echo ""
  echo "==> cleanup: resume outbox (always)"
  if command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}' 2>/dev/null | grep -qx redis-global; then
    docker exec redis-global redis-cli DEL outbox:relay:paused >/dev/null 2>&1 || true
  fi
  local tok="${ADMIN_TOKEN:-}"
  if [[ -z "$tok" ]]; then
    tok=$(login "$ADMIN_EMAIL" "$ADMIN_PASSWORD" 2>/dev/null || true)
  fi
  if [[ -n "$tok" ]]; then
    curl -s -X POST "$HOST/api/v1/lab/outbox/resume" \
      -H "apikey: ${API_KEY}" -H "Authorization: Bearer $tok" >/dev/null || true
    curl -s -X POST "$HOST/api/v1/lab/outbox/relay-once" \
      -H "apikey: ${API_KEY}" -H "Authorization: Bearer $tok" >/dev/null || true
  fi
}
trap cleanup_outbox EXIT

echo "==> ready"
ready=$(curl -s -w "\n%{http_code}" "$HOST/health/ready")
body=$(echo "$ready" | sed '$d')
code=$(echo "$ready" | tail -1)
echo "$body" | python3 -m json.tool 2>/dev/null | head -40 || echo "$body"
[[ "$code" == "200" ]] && ok "health/ready HTTP 200" || bad "health/ready HTTP $code"

echo "==> login admin + buyer"
ADMIN_TOKEN=$(login "$ADMIN_EMAIL" "$ADMIN_PASSWORD")
BUYER_TOKEN=$(login "$BUYER_EMAIL" "$BUYER_PASSWORD")
[[ -n "$ADMIN_TOKEN" ]] && ok "admin login" || bad "admin login"
[[ -n "$BUYER_TOKEN" ]] && ok "buyer login" || bad "buyer login"
AH=(-H "apikey: ${API_KEY}" -H "Authorization: Bearer ${ADMIN_TOKEN}" -H "Content-Type: application/json")
BH=(-H "apikey: ${API_KEY}" -H "Authorization: Bearer ${BUYER_TOKEN}" -H "Content-Type: application/json")

echo "==> failure-matrix"
fm=$(curl -s -o /tmp/fm.json -w "%{http_code}" "$HOST/api/v1/lab/commerce/failure-matrix" "${AH[@]}")
[[ "$fm" == "200" ]] && ok "failure-matrix" || bad "failure-matrix HTTP $fm"

if [[ -z "$PRODUCT_ID" ]]; then
  echo "==> ensure product with stock"
  PRODUCT_ID=$(curl -s -X POST "$HOST/api/v1/products" "${BH[@]}" \
    -d '{"name":"Chaos SKU","description":"chaos","price":1000,"stock":20}' \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])")
  ok "created product id=$PRODUCT_ID"
fi

echo "==> outbox pause → create order → pending → resume"
curl -s -X POST "$HOST/api/v1/lab/outbox/pause" "${AH[@]}" >/dev/null
KEY="chaos-$(date +%s)-$RANDOM"
ORDER=$(curl -s -X POST "$HOST/api/v1/orders" "${BH[@]}" \
  -H "Idempotency-Key: $KEY" \
  -d "{\"items\":[{\"productId\":$PRODUCT_ID,\"qty\":1}]}")
OID=$(echo "$ORDER" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null || true)
OST=$(echo "$ORDER" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['status'])" 2>/dev/null || true)
[[ -n "$OID" ]] && ok "order $OID status=$OST while paused" || bad "order create while paused: $ORDER"

PENDING=$(curl -s "$HOST/api/v1/lab/outbox/pending" "${AH[@]}")
PC=$(echo "$PENDING" | python3 -c "import sys,json; d=json.load(sys.stdin)['data']; print(d.get('pendingCount',0))" 2>/dev/null || echo 0)
PAUSED=$(echo "$PENDING" | python3 -c "import sys,json; d=json.load(sys.stdin)['data']; print(d.get('paused',False))" 2>/dev/null || echo false)
[[ "$PAUSED" == "True" || "$PAUSED" == "true" ]] && ok "outbox paused=true" || bad "outbox paused flag ($PAUSED)"
[[ "$PC" != "" && "$PC" != "0" ]] && ok "pendingCount=$PC" || bad "pendingCount expected >0 got '$PC'"

curl -s -X POST "$HOST/api/v1/lab/outbox/resume" "${AH[@]}" >/dev/null
curl -s -X POST "$HOST/api/v1/lab/outbox/relay-once" "${AH[@]}" >/dev/null
ok "resume + relay-once issued"

echo "==> wait order progress (worker)"
final=""
for i in $(seq 1 30); do
  final=$(curl -s "$HOST/api/v1/orders/$OID" "${BH[@]}" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['status'])" 2>/dev/null || true)
  [[ "$final" == "paid" || "$final" == "payment_failed" || "$final" == "fulfilled" ]] && break
  sleep 1
done
[[ "$final" == "paid" || "$final" == "payment_failed" || "$final" == "fulfilled" ]] \
  && ok "order reached $final" || bad "order stuck status=$final"

echo "==> kafka ping → mongo events"
curl -s -X POST "$HOST/api/v1/lab/kafka/ping" "${AH[@]}" >/dev/null || true
sleep 2
ME=$(curl -s -o /tmp/me.json -w "%{http_code}" "$HOST/api/v1/lab/mongo/events?limit=3" "${AH[@]}")
[[ "$ME" == "200" ]] && ok "mongo events readable" || bad "mongo events HTTP $ME"

echo "==> metrics scrape"
curl -s "$HOST/metrics" | grep -q golangbe_outbox_pending && ok "outbox_pending metric" || bad "missing outbox_pending metric"

echo ""
echo "Result: $pass passed, $fail failed"
echo "Note: EXIT trap resumes outbox. After stopping containers manually, run: ./scripts/lab-restore.sh"
[[ "$fail" -eq 0 ]]
