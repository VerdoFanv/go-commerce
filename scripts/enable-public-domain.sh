#!/usr/bin/env bash
# Enable public domain Ingress + (optional) Let's Encrypt via cert-manager.
# Usage:
#   export DOMAIN=api.yourdomain.com
#   export ACME_EMAIL=you@example.com
#   ./scripts/enable-public-domain.sh
set -euo pipefail

DOMAIN="${DOMAIN:?set DOMAIN=api.yourdomain.com}"
ACME_EMAIL="${ACME_EMAIL:-}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
KUBECTL="${KUBECTL:-sudo k3s kubectl}"

echo "==> Domain: $DOMAIN"

if [[ -n "$ACME_EMAIL" ]]; then
  echo "==> Ensuring cert-manager ClusterIssuers (email=$ACME_EMAIL)"
  tmp="$(mktemp)"
  sed "s/CHANGE_ME@example.com/${ACME_EMAIL}/g" \
    "$ROOT/k8s/cert-manager-letsencrypt.yaml" >"$tmp"
  $KUBECTL apply -f "$tmp"
  rm -f "$tmp"

  echo "==> Applying public Ingress with TLS annotations"
  tmp="$(mktemp)"
  sed "s/\${DOMAIN}/${DOMAIN}/g" "$ROOT/k8s/api-ingress-public.example.yaml" \
    | sed 's/# cert-manager.io\/cluster-issuer:/cert-manager.io\/cluster-issuer:/' \
    | sed 's/# tls:/tls:/' \
    | sed 's/#   - hosts:/  - hosts:/' \
    | sed "s/#       - \${DOMAIN}/      - ${DOMAIN}/" \
    | sed 's/#     secretName:/    secretName:/' \
    >"$tmp"
  # cleaner generation:
  cat >"$tmp" <<EOF
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: api-public
  namespace: golang-be
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: web,websecure
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  ingressClassName: traefik
  tls:
    - hosts:
        - ${DOMAIN}
      secretName: golang-be-tls
  rules:
    - host: ${DOMAIN}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: api
                port:
                  name: http
EOF
  $KUBECTL apply -f "$tmp"
  rm -f "$tmp"
  echo "==> Watch certificate: $KUBECTL -n golang-be get certificate -w"
else
  echo "==> Applying public Ingress (HTTP only — set ACME_EMAIL for TLS)"
  export DOMAIN
  envsubst <"$ROOT/k8s/api-ingress-public.example.yaml" | $KUBECTL apply -f -
fi

echo "==> Done. Test: curl -sS http://${DOMAIN}/health/ready"
echo "    Remember router port-forward 80/443 → this host (see docs/PUBLIC-ACCESS.md)"
