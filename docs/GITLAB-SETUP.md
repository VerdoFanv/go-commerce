# Setup GitLab untuk repo ini

Langkah di akun GitLab kamu (sekali saja).

## 1. Buat project

1. Buka [gitlab.com](https://gitlab.com) → **New project** → **Create blank project** (atau import dari GitHub).
2. Nama: `golang-be` (bebas).
3. Visibility: **Private** (disarankan untuk porto + secrets).

## 2. Push kode ke GitLab

Di mesin lokal (ganti URL):

```bash
cd ~/Project/golang-be
git remote rename origin github   # opsional, kalau masih ada GitHub
git remote add gitlab git@gitlab.com:<username>/golang-be.git
# atau HTTPS:
# git remote add gitlab https://gitlab.com/<username>/golang-be.git

git push -u gitlab HEAD:main
```

Kalau repo baru kosong, pastikan branch default = `main` (Settings → Repository → Default branch).

## 3. Aktifkan Container Registry

1. **Settings → General → Visibility** → pastikan Container Registry **enabled**.
2. Image yang di-push pipeline `release`:
   - `registry.gitlab.com/<username>/golang-be/api:latest`
   - `registry.gitlab.com/<username>/golang-be/worker:latest`

## 4. CI/CD Variables (secrets — JANGAN taruh di kode)

**Settings → CI/CD → Variables → Add variable**

| Key | Value | Flags |
|-----|--------|-------|
| (opsional) `DOCKER_AUTH_CONFIG` | hanya kalau registry eksternal | Masked |

Untuk push ke GitLab Registry, job `release` sudah pakai `CI_REGISTRY_*` bawaan — biasanya **tidak perlu** variable tambahan.

Kalau nanti deploy otomatis ke server (opsional lanjutan):

| Key | Contoh | Protected | Masked |
|-----|--------|-----------|--------|
| `SSH_PRIVATE_KEY` | isi private key deploy | ✅ | ✅ |
| `DEPLOY_HOST` | `192.168.0.155` | ✅ | |
| `K3S_KUBECONFIG` | isi kubeconfig (base64) | ✅ | ✅ |

Jangan simpan password DB/JWT di CI Variables kecuali job deploy memang butuh — runtime secrets tetap di server (`k8s/secret.yaml` / sealed-secrets), bukan di repo.

## 5. Runner

- **GitLab.com Shared Runners**: Settings → CI/CD → Runners → pastikan **Enable shared runners** ON.
- Atau pasang [self-hosted runner](https://docs.gitlab.com/runner/) di Ubuntu lab kalau mau hemat menit CI.

## 6. Cek pipeline

1. Push ke `main` atau buka Merge Request.
2. **Build → Pipelines** — harus hijau: `lint` → `test` → `build` / `docker`.
3. Tag `v0.1.0` → job `release` push image ke registry.

```bash
git tag v0.1.0
git push gitlab v0.1.0
```

## 7. Deploy image GitLab ke k3s (server)

Setelah image ada di registry:

```bash
# Login sekali di server (Personal Access Token dengan scope read_registry)
docker login registry.gitlab.com

# Atau buat imagePullSecret di k3s:
sudo k3s kubectl -n golang-be create secret docker-registry gitlab-registry \
  --docker-server=registry.gitlab.com \
  --docker-username=<gitlab-user> \
  --docker-password=<pat> \
  --docker-email=<email>

# Edit deployment image:
#   registry.gitlab.com/<user>/golang-be/api:latest
#   imagePullPolicy: Always
#   imagePullSecrets: [gitlab-registry]
```

Untuk lab harian, **Option A (build lokal + `k3s ctr import`)** tetap paling simpel (lihat README / PANDUAN).

## 8. Matikan GitHub Actions (opsional)

Folder `.github/workflows/` sudah diganti konsepnya oleh `.gitlab-ci.yml`.  
Kalau remote GitHub masih ada, hapus workflows di GitHub atau archive repo supaya tidak double CI.

## 9. Checklist keamanan

- [ ] Project Private  
- [ ] Tidak ada password production di commit  
- [ ] `k8s/secret.yaml` di `.gitignore`  
- [ ] PAT registry hanya `read_registry` / `write_registry` sesuai kebutuhan  
- [ ] Protected variables hanya di protected branches/tags  
