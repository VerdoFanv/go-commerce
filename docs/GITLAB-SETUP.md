# GitLab setup — two delivery modes

You can use **either** mode (or both):

| Mode | Name | You do | Pipeline does |
| ---- | ---- | ------ | ------------- |
| **A** | Self-deploy | Build + import images on the Ubuntu server, `kubectl apply` / rollout | Optional: only lint/test on push (no deploy) |
| **B** | GitLab CI/CD | Push / tag; click **Play** on deploy when ready | Lint → test → build → Trivy → push registry → (manual) deploy |

Mode A is best for daily lab work. Mode B is best for clean releases and the portfolio “pipeline” story.

---

## One-time: create the GitLab project

1. Open [gitlab.com](https://gitlab.com) → **New project** → blank project (or import).
2. Name: `golang-be`. Visibility: **Private** recommended.
3. Default branch: `main`.

### Push from your Mac

```bash
cd ~/Project/golang-be
git remote add gitlab git@gitlab.com:<username>/golang-be.git
# or: https://gitlab.com/<username>/golang-be.git

git push -u gitlab HEAD:main
```

### Enable CI runners + registry

1. **Settings → CI/CD → Runners** → enable **shared runners** (or install a self-hosted runner on the lab box).
2. **Settings → General → Visibility** → **Container Registry** enabled.
3. Images (Mode B release):
   - `registry.gitlab.com/<username>/golang-be/api:latest`
   - `registry.gitlab.com/<username>/golang-be/worker:latest`

---

## Mode A — Self-deploy (no GitLab deploy required)

Pipeline can still run lint/test when you push — that is fine. You deploy yourself:

```bash
# SSH to the Ubuntu lab box
cd ~/projects/golang-be
git pull   # or rsync from Mac

KAFKA_HOST_ADVERTISE=$(hostname -I | awk '{print $1}') make infra-up   # if needed

docker build --target api    -t golang-be-api:local    .
docker build --target worker -t golang-be-worker:local .
docker save golang-be-api:local    -o /tmp/api.tar
docker save golang-be-worker:local -o /tmp/worker.tar
sudo k3s ctr images import /tmp/api.tar
sudo k3s ctr images import /tmp/worker.tar

sudo k3s kubectl apply -f k8s/namespace.yaml -f k8s/configmap.yaml -f k8s/secret.yaml
sudo k3s kubectl apply -f k8s/api-deployment.yaml -f k8s/api-service.yaml \
  -f k8s/api-hpa.yaml -f k8s/api-ingress.yaml -f k8s/worker-deployment.yaml
sudo k3s kubectl -n golang-be rollout restart deploy/api deploy/worker
sudo k3s kubectl -n golang-be rollout status deploy/api
```

Manifests already use `golang-be-*:local` + `imagePullPolicy: IfNotPresent`.

**You do not need** registry login or CI Variables for Mode A.

---

## Mode B — GitLab CI/CD (build + optional deploy)

### B1. CI Variables (only what you need)

**Settings → CI/CD → Variables**

| Variable | Required for | Notes |
| -------- | ------------ | ----- |
| *(none)* | lint/test/build/release to GitLab Registry | Uses built-in `CI_JOB_TOKEN` / `CI_REGISTRY_*` |
| `SSH_PRIVATE_KEY` | Manual **deploy** job | Deploy user SSH key; **Masked + Protected** |
| `DEPLOY_HOST` | Manual **deploy** job | e.g. `192.168.0.155` or future public IP |
| `DEPLOY_USER` | Manual **deploy** job | e.g. `verdo` |
| `CI_REGISTRY_USER` / `CI_REGISTRY_PASSWORD` | Usually auto | Override only if using an external registry |

Do **not** put DB passwords / JWT in CI Variables unless a job truly needs them. Runtime secrets stay on the server in `k8s/secret.yaml`.

### B2. What the pipeline does

| Job | Stage | When |
| --- | ----- | ---- |
| `lint` / `test` / `build` | every branch & MR | Always |
| `docker` | build + Trivy | Always |
| `release` | push images to GitLab Registry | `main` or tags |
| `deploy` | SSH to server, pull/import, rollout | **manual** (`when: manual`) on `main`/tags |

### B3. Release + deploy flow

```bash
git push gitlab main
# Pipelines → wait for green → open the pipeline → click ▶ on "deploy" when you want

# Or tag a release:
git tag v0.1.0 && git push gitlab v0.1.0
```

### B4. Server prep for Mode B (once)

On the Ubuntu box, allow the GitLab deploy key / your SSH key, and create a pull secret if images are private:

```bash
# Personal Access Token with read_registry
sudo k3s kubectl -n golang-be create secret docker-registry gitlab-registry \
  --docker-server=registry.gitlab.com \
  --docker-username=<gitlab-user> \
  --docker-password=<pat> \
  --docker-email=<email> \
  --dry-run=client -o yaml | sudo k3s kubectl apply -f -
```

When switching manifests from `:local` to registry images, set:

- `image: registry.gitlab.com/<user>/golang-be/api:latest`
- `imagePullPolicy: Always`
- `imagePullSecrets: [{name: gitlab-registry}]`

(or keep Mode A local tags and let the deploy job import tarballs — see `scripts/deploy-from-registry.sh`).

---

## Switching between modes

- **Daily coding:** Mode A (fast feedback on LAN).
- **Show portfolio / tag release:** Mode B (green pipeline + registry images).
- You can push to GitLab every day (Mode B lint/test) while still self-deploying with Mode A — no conflict.

---

## Security checklist

- [ ] Project Private
- [ ] `k8s/secret.yaml` never committed
- [ ] Deploy key / PAT scoped minimally
- [ ] Protected + Masked variables for secrets
- [ ] Rotate lab API keys if the box becomes public ([PUBLIC-ACCESS.md](PUBLIC-ACCESS.md))
