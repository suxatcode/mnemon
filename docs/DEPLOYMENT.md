# Development and Deployment

## Local Development

Prerequisites:

- Go 1.25.3 (required by `jackc/pgx/v5 v5.11.0`; `go.mod` pins `go 1.25.0` / `toolchain go1.25.3`)
- `make`
- `jq` for the E2E test script

Common commands:

```bash
make deps
make build
make unit
make test
make install
```

Use a project-local data directory when testing manually:

```bash
MNEMON_DATA_DIR=.mnemon-dev ./mnemon store create default
MNEMON_DATA_DIR=.mnemon-dev ./mnemon remember --no-diff "Local development memory" --cat fact --imp 3
MNEMON_DATA_DIR=.mnemon-dev ./mnemon recall "development memory"
```

## Container Development

Create a local environment file:

```bash
cp .env.example .env
```

Start a shell inside the Go development image:

```bash
make compose-dev
```

Inside the container:

```bash
make build
make test
```

## Container Deployment

Build the runtime image:

```bash
make docker-build
```

Run one command with persistent data mounted at `/mnemon`:

```bash
docker run --rm \
  -v mnemon-data:/mnemon \
  --env-file .env \
  mnemon-dev/mnemon:dev status
```

Or use Docker Compose:

```bash
cp .env.example .env
make compose-up
docker compose run --rm mnemon recall "query"
make compose-down
```

## Optional Embeddings

Mnemon works without embeddings. To use Ollama-backed vector search in the Compose environment:

```bash
docker compose --profile embeddings up -d ollama
docker compose exec ollama ollama pull nomic-embed-text
docker compose run --rm mnemon embed "hello"
```

The relevant environment variables are:

- `MNEMON_EMBED_ENDPOINT`
- `MNEMON_EMBED_MODEL`

For host-based Ollama, set `MNEMON_EMBED_ENDPOINT=http://host.docker.internal:11434` on Docker Desktop, or use the host gateway address for Linux deployments.

## Team memory gateway

Local `mnemon` without a remote stays SQLite (`~/.mnemon`, cap 1000). Helm/AWS runs `mnemon-server` against **external** Postgres. This chart never deploys a Postgres StatefulSet or PVC. The only in-chart volume is optional SQLite.

TLS defaults to in-pod (HTTPS JSON) for bring-up without an Ingress. Set `server.tls.enabled=false` when an edge proxy terminates HTTPS (nginx Ingress, Istio Gateway, ALB) and forwards HTTP. The Service port is named `https` or `http` to match. JWTs are self-issued HS256; the signing key can be chart-generated or an existing Secret (`server.existingSecret`) from External Secrets / AWS Secrets Manager — TLS is independent (`server.tls.existingSecret`, cert-manager, or generated `*-tls`).

Recall on a team remote is fully shared (personal notes are write-isolated, not read-isolated). `mnemon link` is not owner-scoped: it may connect another teammate's memory to yours. Forget / GC / `--keep` are owner-scoped.

Chart defaults (internal bring-up):

- SQLite, `replicaCount` forced to 1, no PVC unless `persistence.enabled`
- JWT signing key generated in `mnemon-app` (`fullnameOverride` defaults to `mnemon`; or `server.existingSecret`)
- In-pod TLS generated in the same secret, or `{release}-tls` when JWT comes from an existing Secret
- Probes on `GET /health` and `GET /ready`
- `maxInsights: 25000` per principal (personal layer only)
- `MNEMON_DATA_DIR=/data` in the pod, so `mnemon-server user issue` uses the same SQLite path as `serve` without passing `--data-dir`. Postgres issue uses `MNEMON_DATABASE_URL`.

First-class production knobs:

| Value | Purpose |
|---|---|
| `hostname` | DNS name for Ingress host, cert-manager `dnsNames`, and NOTES `--server` |
| `ingress.enabled` / `ingress.className` | Kubernetes Ingress; class is `nginx`, `alb`, `istio`, … — not hardcoded |
| `certManager.*` | Optional Certificate CR; does not install cert-manager. Set `issuerName` / `issuerGroup` |
| `server.tls.enabled` | In-pod TLS. `false` for edge TLS (Istio/Ingress) |
| `database.url` / `database.existingSecret` | External Postgres DSN. Required for HA (`replicaCount` > 1) |
| `image.pullSecrets` | Private registry pull (not needed for the public GHCR image) |
| `image.repository` | Default: `ghcr.io/suxatcode/mnemon-server` (`:dev`) |
| `fullnameOverride` | Default: `mnemon` so Service/Deployment are not `mnemon-mnemon-server` |

Issue a user (takes effect immediately, no pod restart):

```bash
kubectl exec deploy/mnemon -- \
  mnemon-server user issue \
    --principal alice@team --role user \
    --server mnemon.example.com \
    --server-name mnemon.example.com \
    --jwt-key /config/jwt.key \
    --out -
```

When the chart generated in-pod TLS, also pass `--ca-file /config/ca.crt`. Public Ingress/Let's Encrypt does not need that. Issue prints the store path on stderr. On the client: `mnemon auth login --default invite.json`. Use `mnemon --local ...` only to bypass the team store.

### Container image and Helm chart (GHCR)

Prefer **GHCR** over Docker Hub (`docker.io`):

- Image: `ghcr.io/suxatcode/mnemon-server` (linux/amd64 + linux/arm64)
- Chart: `oci://ghcr.io/suxatcode/charts/mnemon-server` (Helm 3.8+)
- GitHub Packages is free for public artifacts, uses `GITHUB_TOKEN`, and avoids Docker Hub anonymous pull rate limits on CI/Kubernetes.
- The Go module path stays `github.com/mnemon-dev/mnemon`; image and chart live under the fork that publishes them.
- `.github/workflows/image.yml` publishes both on pushes to `feat/remote-gateway`, `workflow_dispatch`, and `v*` tags. Chart version comes from `Chart.yaml` (`0.1.1`); git tags `v*` override that version. After the first chart push, make the **charts/mnemon-server** package public under GitHub → Packages (same one-way Danger Zone as the image). Minikube still loads a local `mnemon-dev/mnemon-server` tag and `--set`s `image.repository`.

```bash
# After this branch is pushed:
gh workflow run image.yml --ref feat/remote-gateway
# optional: -f tag=dev   (image only; chart version is Chart.yaml / git tag)

helm install mnemon oci://ghcr.io/suxatcode/charts/mnemon-server --version 0.1.1
# or: helm upgrade --install mnemon oci://ghcr.io/suxatcode/charts/mnemon-server --version 0.1.1
```

The chart defaults `image.repository` to the public GHCR image. `image.pullSecrets` is only needed for a private package. There is no GitHub Pages `helm repo add` index; OCI is the published Helm repo.

### Minikube integration suite

The suite uses a dedicated profile (`mnemon-gateway`) and does not switch your current kubectl context for other commands. It deploys **its own** Postgres Deployment (`ext-pg`) for tests and points Helm at `database.existingSecret`. It covers that DSN, `database.url`, `values-rds.yaml`, `values-istio.yaml` (HTTP, port name `http`, no Ingress), JWT `existingSecret`, Ingress class + hostname, SQLite PVC restart, TLS off, and operator flows (concurrent writes, replica kill, token TTL, GC prune, forged import owner).

```bash
make test-minikube
```

Optional env: `MINIKUBE_PROFILE`, `SCENARIOS` (comma list: `postgres,url,rds,jwt-secret,istio,sqlite,tls-off`; `bundled`/`external` alias `postgres`), `IMAGE_TAG` (default unique `dev-<timestamp>`), `SKIP_BUILD=1`, `KEEP_CLUSTER=0` (delete the profile at the end).

### Amazon RDS, AWS Ingress, and Istio

Overlays (`values-rds.yaml`, `values-aws.yaml`, `values-istio.yaml`) ship inside the chart. Pull it, then `-f` the overlay:

```bash
helm pull oci://ghcr.io/suxatcode/charts/mnemon-server --version 0.1.1 --untar
```

DSN-only overlay:

```bash
helm upgrade --install mnemon ./mnemon-server \
  -f ./mnemon-server/values-rds.yaml
```

AWS overlay (RDS DSN secret, JWT existingSecret, Ingress + cert-manager, **HTTP pods**, edge TLS):

```bash
helm upgrade --install mnemon ./mnemon-server \
  -f ./mnemon-server/values-aws.yaml \
  --set hostname=mnemon.example.com \
  --set ingress.className=nginx
```

Istio / mesh overlay (no in-chart Ingress or Certificate; pods HTTP on port `8080` named `http`):

```bash
helm upgrade --install mnemon ./mnemon-server \
  -f ./mnemon-server/values-istio.yaml \
  --set hostname=mnemon.example.com
```

Point your existing Gateway/VirtualService at Service `mnemon:8080`. The chart does not install Istio CRDs.

Create `mnemon-db` (`url`) and, for AWS, `mnemon-jwt` (`jwt.key`, ≥32 bytes) first — typically via External Secrets from AWS Secrets Manager. `values-aws.yaml` expects cert-manager to write `mnemon-tls` for the Ingress. Postgres backup/restore stays with RDS; this chart does not ship a backup job. There is no `users.json`.

The chart creates a non-root ServiceAccount (`automountServiceAccountToken: false`), runs UID/GID 65532 (matching the image), and emits a PodDisruptionBudget when `replicaCount` > 1. Optional HPA: `--set autoscaling.enabled=true` (Postgres only; needs metrics-server).

### Issue, revoke, and JWT rotation

Issue from the pod (NOTES prints the exact command) or from a laptop that can reach Postgres:

```bash
mnemon-server jwt keygen --out jwt.key   # once; store in mnemon-jwt

mnemon-server user issue \
  --database-url "$MNEMON_DATABASE_URL" \
  --jwt-key ./jwt.key \
  --principal alice@team --role user \
  --server mnemon.example.com \
  --out invite.json

mnemon auth login --default invite.json
```

Revoke every token for one principal without rotating the HMAC key:

```bash
kubectl exec deploy/mnemon -- mnemon-server user revoke --principal bob@team
# or, with the DSN on a workstation:
mnemon-server user revoke --database-url "$MNEMON_DATABASE_URL" --principal bob@team
```

Rotate the signing key when it may be leaked:

1. Generate a new key (`mnemon-server jwt keygen --out jwt.key`) and write it to `mnemon-jwt` / External Secrets (`jwt.key`, ≥32 bytes).
2. Restart the server pods so they mount the new key (`kubectl rollout restart deploy/mnemon`). Helm does not restart on an out-of-band Secret mutation.
3. Every existing JWT fails signature checks immediately. Re-issue each principal with `user issue` as above.

Do not keep the old HMAC key in the cluster after rotation.

## Release Deployment

Tagged releases are handled by GoReleaser through `.github/workflows/release.yml`.

Required repository secret:

- `HOMEBREW_TAP_TOKEN`, only needed for publishing the Homebrew tap

Create a local snapshot build without publishing:

```bash
make release-snapshot
```
