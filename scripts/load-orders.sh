#!/usr/bin/env bash
# Parallel order creates against one SKU — stock must never go negative.
# Asserts created <= initial_stock via GET /products/:id (seller token).
#
# Usage:
#   API=http://192.168.0.155 API_KEY=... TOKEN=... SELLER_TOKEN=... PRODUCT_ID=1 N=40 ./scripts/load-orders.sh
set -euo pipefail

API="${API:-http://127.0.0.1:8080}"
API_KEY="${API_KEY:?set API_KEY}"
TOKEN="${TOKEN:?set TOKEN (buyer access token)}"
SELLER_TOKEN="${SELLER_TOKEN:-$TOKEN}"
PRODUCT_ID="${PRODUCT_ID:?set PRODUCT_ID}"
N="${N:-40}"
QTY="${QTY:-1}"

stock_before=$(curl -s "$API/api/v1/products/$PRODUCT_ID" \
  -H "apikey: $API_KEY" -H "Authorization: Bearer $SELLER_TOKEN" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['stock'])")

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

echo "stock_before=$stock_before — firing $N parallel POST /orders (product=$PRODUCT_ID qty=$QTY)..."

for i in $(seq 1 "$N"); do
  (
    code=$(curl -s -o "$tmpdir/body-$i.json" -w "%{http_code}" \
      -X POST "$API/api/v1/orders" \
      -H "apikey: $API_KEY" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Idempotency-Key: load-$i-$(date +%s%N)" \
      -H "Content-Type: application/json" \
      -d "{\"items\":[{\"productId\":$PRODUCT_ID,\"qty\":$QTY}]}")
    echo "$code" > "$tmpdir/code-$i.txt"
  ) &
done
wait

ok=0
conflict=0
other=0
for i in $(seq 1 "$N"); do
  c=$(cat "$tmpdir/code-$i.txt")
  case "$c" in
    201) ok=$((ok+1)) ;;
    409) conflict=$((conflict+1)) ;;
    *) other=$((other+1)); echo "unexpected $c: $(cat "$tmpdir/body-$i.json")" ;;
  esac
done

stock_after=$(curl -s "$API/api/v1/products/$PRODUCT_ID" \
  -H "apikey: $API_KEY" -H "Authorization: Bearer $SELLER_TOKEN" \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['stock'])")

echo "created=$ok conflict=$conflict other=$other stock_before=$stock_before stock_after=$stock_after"

if [[ "$stock_after" -lt 0 ]]; then
  echo "FAIL: oversell — stock_after < 0" >&2
  exit 1
fi
if [[ "$ok" -gt "$stock_before" ]]; then
  echo "FAIL: created ($ok) > stock_before ($stock_before)" >&2
  exit 1
fi
if [[ "$other" -ne 0 ]]; then
  echo "FAIL: unexpected statuses=$other" >&2
  exit 1
fi
echo "PASS: no oversell; created<=stock_before; stock_after>=0"
