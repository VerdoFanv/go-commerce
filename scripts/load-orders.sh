#!/usr/bin/env bash
# Parallel order creates against one SKU — stock must never go negative.
# Usage:
#   API=http://192.168.0.155 API_KEY=... TOKEN=... PRODUCT_ID=1 ./scripts/load-orders.sh
set -euo pipefail

API="${API:-http://127.0.0.1:8080}"
API_KEY="${API_KEY:?set API_KEY}"
TOKEN="${TOKEN:?set TOKEN (buyer access token)}"
PRODUCT_ID="${PRODUCT_ID:?set PRODUCT_ID}"
N="${N:-20}"
QTY="${QTY:-1}"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

echo "Firing $N parallel POST /orders (product=$PRODUCT_ID qty=$QTY)..."

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

echo "created=$ok conflict=$conflict other=$other"
echo "Check stock: SELECT id,name,stock FROM products WHERE id=$PRODUCT_ID;  (must be >= 0)"
