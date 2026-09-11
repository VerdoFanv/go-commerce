#!/usr/bin/env bash
# Install LitmusChaos operator (cluster) + experiment CRs (golang-be ns).
#
# Usage (on the lab box that has k3s):
#   ./scripts/litmus-install.sh
#   SUDO_PASS='…' ./scripts/litmus-install.sh
#
# Idempotent. Does NOT install ChaosCenter UI (too heavy for single-node APU).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NS_APP="${NS_APP:-golang-be}"
NS_LITMUS="${NS_LITMUS:-litmus}"
# Pinned operator manifest from litmus 3.31.0 docs (operator-only, no portal).
OPERATOR_URL="${OPERATOR_URL:-https://raw.githubusercontent.com/litmuschaos/litmus/3.31.0/mkdocs/docs/litmus-operator-latest.yaml}"
WAIT_SEC="${WAIT_SEC:-180}"

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

if ! command -v k3s >/dev/null 2>&1 && [[ "$(id -u)" -ne 0 ]]; then
  if ! SUDO k3s kubectl version --client >/dev/null 2>&1; then
    echo "k3s not available on this host. Run litmus-install on the lab box." >&2
    exit 1
  fi
fi

echo "==> ensuring namespace $NS_APP"
K apply -f "$ROOT/k8s/namespace.yaml"

echo "==> installing LitmusChaos operator ($OPERATOR_URL)"
TMP="$(mktemp)"
curl -fsSL "$OPERATOR_URL" -o "$TMP"
K apply -f "$TMP"
rm -f "$TMP"

echo "==> waiting for chaos-operator-ce in $NS_LITMUS"
deadline=$((SECONDS + WAIT_SEC))
while true; do
  ready=$(K -n "$NS_LITMUS" get deploy chaos-operator-ce -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)
  if [[ "${ready:-0}" -ge 1 ]]; then
    echo "operator ready (replicas=$ready)"
    break
  fi
  if (( SECONDS >= deadline )); then
    echo "timeout waiting for chaos-operator-ce" >&2
    K -n "$NS_LITMUS" get pods,deploy 2>&1 || true
    exit 1
  fi
  sleep 3
done

echo "==> applying app RBAC + ChaosExperiment in $NS_APP"
K apply -f "$ROOT/chaos/rbac.yaml"
K apply -f "$ROOT/chaos/experiments/pod-delete.yaml"

echo "==> pre-pull go-runner image on node (best-effort)"
# k3s shares containerd with the node; import via crictl if present.
if SUDO crictl pull docker.io/litmuschaos/go-runner:3.16.0 >/dev/null 2>&1; then
  echo "pulled litmuschaos/go-runner:3.16.0"
else
  echo "WARN: could not pre-pull go-runner (experiment will pull on first run)" >&2
fi

echo ""
echo "Litmus install OK."
echo "Run experiments:  make chaos"
echo "Or:               ./scripts/litmus-run.sh"
