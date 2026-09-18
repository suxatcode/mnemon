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

Local `mnemon` without a remote stays SQLite (`~/.mnemon`, cap 1000). Helm/AWS runs `mnemon-server` against Postgres. TLS is on by default (HTTPS JSON). Set `server.tls.enabled=false` for HTTP. JWTs are self-issued HS256; the signing key can be chart-generated or an existing Secret (`server.existingSecret`) from External Secrets / AWS Secrets Manager — TLS is independent (`server.tls.existingSecret` or a generated `*-tls` secret).

Recall on a team remote is fully shared (personal notes are write-isolated, not read-isolated). `mnemon link` is not owner-scoped; forget / GC / `--keep` are.

Chart defaults (internal bring-up):

- Bundled Postgres StatefulSet
- `replicaCount: 2`
- JWT signing key generated in `{release}-app` (or `server.existingSecret`)
- TLS generated in the same secret, or `{release}-tls` when JWT comes from an existing Secret
- Probes on `GET /health` and `GET /ready`
- `maxInsights: 25000` per principal (personal layer only)
- `MNEMON_DATA_DIR=/data` in the pod, so `mnemon-server user issue` uses the same SQLite path as `serve` without passing `--data-dir`. Postgres issue uses `MNEMON_DATABASE_URL`.

Issue a user (takes effect immediately, no pod restart):

```bash
kubectl exec deploy/mnemon -- \
  mnemon-server user issue \
    --principal alice@team --role user \
    --server mnemon.example.com:7443 \
    --server-name mnemon.example.com \
    --jwt-key /config/jwt.key \
    --ca-file /config/ca.crt \
    --out -
```

Issue prints the store path on stderr. On the client: `mnemon auth login --default invite.json`. Use `mnemon --local ...` only to bypass the team store.

### Minikube integration suite

The suite uses a dedicated profile (`mnemon-gateway`) and does not switch your current kubectl context for other commands. It covers bundled Postgres, external DSN / `database.url` / `values-rds.yaml`, JWT `existingSecret`, Ingress objects, SQLite PVC restart, TLS off, and operator flows (concurrent writes, replica kill, token TTL, GC prune, forged import owner).

```bash
make test-minikube
```

Optional env: `MINIKUBE_PROFILE`, `SCENARIOS` (comma list: `bundled,external,rds,postgres,jwt-secret,ingress,sqlite,tls-off`), `IMAGE_TAG` (default unique `dev-<timestamp>`), `SKIP_BUILD=1`, `KEEP_CLUSTER=0` (delete the profile at the end).

### Amazon RDS and AWS secrets (production)

DSN-only overlay:

```bash
helm upgrade --install mnemon deploy/helm/mnemon-server \
  -f deploy/helm/mnemon-server/values-rds.yaml \
  --set image.tag=dev
```

Full AWS overlay (RDS DSN secret, JWT existingSecret, Ingress + cert-manager Certificate):

```bash
helm upgrade --install mnemon deploy/helm/mnemon-server \
  -f deploy/helm/mnemon-server/values-aws.yaml \
  --set image.tag=dev
```

Create `mnemon-db` (`url`) and `mnemon-jwt` (`jwt.key`, ≥32 bytes) first — typically via External Secrets from AWS Secrets Manager. `values-aws.yaml` expects cert-manager to write `mnemon-tls`. Postgres backup/restore stays with RDS; this chart does not ship a backup job.

`values-rds.yaml` / `values-aws.yaml` set `postgresql.enabled: false`. Principals and token `jti` rows live in that Postgres; there is no `users.json`.

## Release Deployment

Tagged releases are handled by GoReleaser through `.github/workflows/release.yml`.

Required repository secret:

- `HOMEBREW_TAP_TOKEN`, only needed for publishing the Homebrew tap

Create a local snapshot build without publishing:

```bash
make release-snapshot
```
