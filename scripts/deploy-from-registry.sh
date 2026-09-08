#!/usr/bin/env bash
# Mode B helper: pull GitLab registry images on the server and rollout.
# Usage (on the Ubuntu box, after docker login registry.gitlab.com):
#   export CI_REGISTRY_IMAGE=registry.gitlab.com/<user>/golang-be
#   export TAG=latest
#   ./scripts/deploy-from-registry.sh
set -euo pipefail

CI_REGISTRY_IMAGE="${CI_REGISTRY_IMAGE:?set CI_REGISTRY_IMAGE}"
TAG="${TAG:-latest}"
NS="${NS:-golang-be}"
KUBECTL="${KUBECTL:-sudo k3s kubectl}"

API_IMAGE="${CI_REGISTRY_IMAGE}/api:${TAG}"
WORKER_IMAGE="${CI_REGISTRY_IMAGE}/worker:${TAG}"

echo "==> Pulling $API_IMAGE"
docker pull "$API_IMAGE"
echo "==> Pulling $WORKER_IMAGE"
docker pull "$WORKER_IMAGE"

# Retag for local import (works even without imagePullSecrets)
docker tag "$API_IMAGE" golang-be-api:local
docker tag "$WORKER_IMAGE" golang-be-worker:local
docker save golang-be-api:local -o /tmp/golang-be-api.tar
docker save golang-be-worker:local -o /tmp/golang-be-worker.tar
sudo k3s ctr images import /tmp/golang-be-api.tar
sudo k3s ctr images import /tmp/golang-be-worker.tar
rm -f /tmp/golang-be-api.tar /tmp/golang-be-worker.tar

sudo k3s kubectl -n "$NS" rollout restart deploy/api deploy/worker
sudo k3s kubectl -n "$NS" rollout status deploy/api --timeout=180s
sudo k3s kubectl -n "$NS" rollout status deploy/worker --timeout=120s
sudo k3s kubectl -n "$NS" get pods
echo "==> Deploy complete"