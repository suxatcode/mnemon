# Development and Deployment

## Local Development

Prerequisites:

- Go 1.24.6 or newer in the 1.24 series (the server image builds with Go 1.25.3 because `jackc/pgx/v5 v5.11.0` requires it)
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

Local `mnemon` without a remote stays SQLite (`~/.mnemon`, cap 1000). Helm/AWS runs `mnemon-server` against Postgres with HTTPS JSON and self-issued JWTs.

Build the server image:

```bash
make docker-build-server
```

Chart defaults (internal bring-up):

- Bundled Postgres StatefulSet
- `replicaCount: 2`
- JWT signing key and TLS generated in the app secret
- Probes on `GET /health` and `GET /ready`
- `maxInsights: 25000` per principal (personal layer only)

Issue a user (takes effect immediately, no pod restart):

```bash
kubectl exec deploy/mnemon -- \
  mnemon-server user issue \
    --principal alice@team --role user \
    --server mnemon.example.com:7443 \
    --server-name mnemon.example.com \
    --jwt-key /config/jwt.key \
    --ca-file /config/ca.crt \
    --data-dir /data \
    --out -
```

`--data-dir /data` matches the server container (and `MNEMON_DATA_DIR`); it is required for SQLite so issue/revoke hit the same database as `serve`. Postgres issue uses `MNEMON_DATABASE_URL` from the pod env.

On the client: `mnemon auth login --default invite.json`. Use `mnemon --local ...` only to bypass the team store.

### Minikube integration suite

The suite uses a dedicated profile (`mnemon-gateway`) and does not switch your current kubectl context for other commands. It covers bundled Postgres, external DSN / `values-rds.yaml`, SQLite PVC restart, TLS off, and the main user/operator flows.

```bash
make test-minikube
```

Optional env: `MINIKUBE_PROFILE`, `SCENARIOS` (comma list: `bundled,external,rds,sqlite,tls-off`), `SKIP_BUILD=1`, `KEEP_CLUSTER=0` (delete the profile at the end).

### Amazon RDS (production)

Use the RDS overlay and a DSN secret (Secrets Manager / ExternalSecret):

```bash
helm upgrade --install mnemon deploy/helm/mnemon-server \
  -f deploy/helm/mnemon-server/values-rds.yaml \
  --set image.tag=dev
```

`values-rds.yaml` sets `postgresql.enabled: false` and reads `database.existingSecret`. Principals and token `jti` rows live in that Postgres; there is no `users.json`.

## Release Deployment

Tagged releases are handled by GoReleaser through `.github/workflows/release.yml`.

Required repository secret:

- `HOMEBREW_TAP_TOKEN`, only needed for publishing the Homebrew tap

Create a local snapshot build without publishing:

```bash
make release-snapshot
```
