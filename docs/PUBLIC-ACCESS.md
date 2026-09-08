# Public IP & domain access

Goal: from **LAN-only** (`192.168.0.155`) → later **public IP + HTTPS domain**, without redesigning the app.

Traefik on k3s already listens on **80** and **443**. This guide prepares the box and documents the cutover.

---

## What is already prepared on the lab server

- Traefik LoadBalancer on node IP: ports **80** and **443**
- Ingress serves LAN IP and `golang-be.local`
- UFW allows **22 / 80 / 443** (SSH + HTTP/HTTPS)
- Example public ingress: [`k8s/api-ingress-public.example.yaml`](../k8s/api-ingress-public.example.yaml)
- Helper script: [`scripts/enable-public-domain.sh`](../scripts/enable-public-domain.sh)

You still need: **router port-forward** (or public IP on the box) + **DNS A record** + (recommended) **TLS certificate**.

---

## Architecture (later)

```text
Internet
   │
   │  DNS: api.yourdomain.com → PUBLIC_IP
   ▼
Router / cloud firewall
   │  forward TCP 80,443 → 192.168.0.155
   ▼
Ubuntu lab (k3s Traefik :80/:443)
   │
   ▼
Ingress → Service api → Pods
```

Laptop on the same LAN can keep using `http://192.168.0.155/` or `golang-be.local`.

---

## Step 1 — Home router / network (when you have public IP)

1. Note the lab box LAN IP (today: `192.168.0.155`). Prefer a **DHCP reservation** so it does not change.
2. On the router: **port forward**
   - WAN TCP **80** → `192.168.0.155:80`
   - WAN TCP **443** → `192.168.0.155:443`
3. If the ISP gives a **public IP on the WAN**, that is your `PUBLIC_IP`.
4. If you are behind CGNAT (no real public IP), use a tunnel (Cloudflare Tunnel, Tailscale Funnel, etc.) instead of raw port-forward.

**Do not** forward Postgres `5432`, Redis, Mongo, or Kafka to the internet.

---

## Step 2 — DNS

Create an **A record**:

```text
api.yourdomain.com    A    <PUBLIC_IP>
```

Optional:

```text
yourdomain.com        A    <PUBLIC_IP>
www.yourdomain.com    CNAME api.yourdomain.com
```

Wait for DNS propagation (`dig +short api.yourdomain.com`).

---

## Step 3 — Apply public Ingress (+ TLS)

### Option A — HTTP first (smoke test)

```bash
export DOMAIN=api.yourdomain.com
envsubst < k8s/api-ingress-public.example.yaml | sudo k3s kubectl apply -f -
curl -s http://$DOMAIN/health/ready
```

### Option B — HTTPS with cert-manager + Let's Encrypt

If cert-manager is installed (see below):

```bash
export DOMAIN=api.yourdomain.com
export ACME_EMAIL=you@example.com
./scripts/enable-public-domain.sh
```

Then:

```bash
curl -s https://$DOMAIN/health/ready
```

---

## Step 4 — Client / laptop

- From internet: `https://api.yourdomain.com`
- From LAN: still `http://192.168.0.155` (or hosts entry for the domain pointing at LAN IP — optional split DNS)

Update mobile/web clients to the domain URL and keep sending `apikey` + JWT.

---

## Security before going public

- [ ] Rotate `API_KEY` and `JWT_SECRET` (no lab defaults)
- [ ] Strong DB/Mongo passwords; no DB ports on WAN
- [ ] UFW: only 22/80/443 (SSH ideally key-only + fail2ban)
- [ ] Prefer HTTPS only once cert works; redirect HTTP→HTTPS via Traefik middleware if desired
- [ ] Rate limit already on; consider lowering `RATE_LIMIT_MAX` for public exposure

---

## Install cert-manager (one-time, optional until you have a domain)

On the Ubuntu box:

```bash
sudo k3s kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml
sudo k3s kubectl -n cert-manager rollout status deploy/cert-manager --timeout=120s
sudo k3s kubectl apply -f k8s/cert-manager-letsencrypt.yaml
```

Edit `k8s/cert-manager-letsencrypt.yaml` email before applying (or use the script).

---

## Rollback to LAN-only

```bash
sudo k3s kubectl apply -f k8s/api-ingress.yaml
sudo k3s kubectl delete -f k8s/api-ingress-public.example.yaml --ignore-not-found
# remove router port-forwards when done testing
```
