#!/usr/bin/env bash
# Full lab deploy on a single Ubuntu box:
#   Docker Compose  = Kafka + Typesense (+ shared Postgres/Redis/Mongo already up)
#   k3s             = api + worker pods
#
# Usage (on the server, from repo root):
#   ./scripts/deploy-k3s-lab.sh
#
# Prereq: k3s installed, shared-net + infra-db running, k8s/secret.yaml filled.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

HOST_IP="${HOST_IP:-$(hostname -I | awk '{print $1}')}"
echo "==> HOST_IP=${HOST_IP}"

if [[ ! -f k8s/secret.yaml ]]; then
  echo "missing k8s/secret.yaml — copy from secret.example.yaml and fill values" >&2
  exit 1
fi

echo "==> data plane: kafka + typesense"
KAFKA_HOST_ADVERTISE="${HOST_IP}" docker compose up -d kafka typesense

echo "==> build images"
docker build --target api -t golang-be-api:local .
docker build --target worker -t golang-be-worker:local .

echo "==> import into k3s containerd"
docker save golang-be-api:local -o /tmp/golang-be-api.tar
docker save golang-be-worker:local -o /tmp/golang-be-worker.tar
sudo k3s ctr images import /tmp/golang-be-api.tar
sudo k3s ctr images import /tmp/golang-be-worker.tar

echo "==> render ConfigMap (HOST_IP → ${HOST_IP})"
TMP_CM="$(mktemp)"
sed "s/HOST_IP/${HOST_IP}/g" k8s/configmap.yaml >"$TMP_CM"

echo "==> apply manifests"
sudo k3s kubectl apply -f k8s/namespace.yaml
sudo k3s kubectl apply -f "$TMP_CM" -f k8s/secret.yaml
sudo k3s kubectl apply \
  -f k8s/api-deployment.yaml \
  -f k8s/api-service.yaml \
  -f k8s/api-hpa.yaml \
  -f k8s/api-ingress.yaml \
  -f k8s/worker-deployment.yaml
rm -f "$TMP_CM"

echo "==> rollout"
sudo k3s kubectl -n golang-be rollout restart deploy/api deploy/worker
sudo k3s kubectl -n golang-be rollout status deploy/api --timeout=180s
sudo k3s kubectl -n golang-be rollout status deploy/worker --timeout=180s
sudo k3s kubectl -n golang-be get pods,svc,ingress

echo "==> done. Probe: curl -s http://${HOST_IP}/health/ready"
