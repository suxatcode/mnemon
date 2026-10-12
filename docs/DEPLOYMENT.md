# Development and Deployment

## Local CLI

Go 1.25.3, `make`, `jq` (for E2E).

```bash
make deps && make build && make unit && make test && make install
MNEMON_DATA_DIR=.mnemon-dev ./mnemon store create default
```

Compose / embeddings: `.env.example`, `make compose-dev`. Optional: `MNEMON_EMBED_ENDPOINT`, `MNEMON_EMBED_MODEL`.

## Hosting mnemon-server

Image: `ghcr.io/suxatcode/mnemon-server`  
Chart: `oci://ghcr.io/suxatcode/charts/mnemon-server`  
Resources are named `mnemon`. Helm never deploys Postgres.

| Track | Image | Chart |
|---|---|---|
| Dev (`main`) | `:dev` and `:<sha>` | git checkout or image `:dev` |
| Production | `:1.0.0` (git tag `v1.0.0`) | `--version 1.0.0` (pins the image tag) |

### Dev (default)

SQLite, one replica, in-pod TLS, chart-generated JWT. Data is `emptyDir` unless `persistence.enabled=true` (single-node PVC, not HA).

```bash
helm install mnemon deploy/helm/mnemon-server
make test-minikube
kubectl exec deploy/mnemon -- mnemon-server user issue \
  --principal alice@team --role user --server mnemon.example.com \
  --jwt-key /config/jwt.key --ca-file /config/ca.crt --out -
mnemon auth login --default invite.json
```

### Production

**Persistence:** external Postgres (`database.existingSecret`, key `url`). No in-chart PVC. Backups are RDS/Cloud SQL snapshots. Do not run production on SQLite.

**Image:** `helm install ... --version 1.0.0`. That chart sets `image.tag=1.0.0`. Do not use `:dev`.

**TLS:** terminate at Ingress/mesh (`server.tls.enabled=false`). Overlays in the chart tarball: `values-rds.yaml`, `values-aws.yaml`, `values-istio.yaml`.

```bash
helm pull oci://ghcr.io/suxatcode/charts/mnemon-server --version 1.0.0 --untar
# Secrets first: mnemon-db (url), mnemon-jwt (jwt.key). Then:
helm upgrade --install mnemon ./mnemon-server -f ./mnemon-server/values-aws.yaml \
  --set hostname=mnemon.example.com --set ingress.className=nginx
```

**JWT:** create `mnemon-jwt` before install. Do not use the chart-generated key.

```bash
mnemon-server jwt keygen --out jwt.key    # once; ≥32 bytes
mnemon-server user issue --database-url "$MNEMON_DATABASE_URL" --jwt-key ./jwt.key \
  --principal alice@team --role user --server mnemon.example.com --out invite.json
mnemon-server user revoke --database-url "$MNEMON_DATABASE_URL" --principal bob@team
```

Rotation is a flag day: new `jwt.key` → restart pods → re-issue everyone. Revoke is per-principal and does not rotate the key.

**OIDC (optional).** Invite JWTs stay the default. Set `oidc.enabled` to also accept RS256/ES256 **access tokens** from any OpenID provider. The token `aud` must be the API audience (not the CLI client id). ID tokens and opaque tokens are rejected. The access-token lifetime is the logout window: the IdP session ending does not revoke a token the gateway already accepted. `mnemon-server user revoke` does.

Auto provision (`oidc.provision=auto`, the default) creates a `user` principal on first valid token, and it refuses to start unless `oidc.allowClaim` and `oidc.allowValue` name a group or role the token must carry. `provision=allowlist` admits only rows created with `mnemon-server user add`. A disabled principal is always rejected.

```yaml
oidc:
  enabled: true
  issuer: https://idp.example/realms/mnemon
  audience: mnemon
  jwksUrl: http://idp.internal/jwks   # optional, when the pod cannot fetch the public issuer
  allowClaim: groups
  allowValue: mnemon-users
```

`--oidc-jwks-url` is the in-cluster JWKS. Token `iss` stays the browser-facing issuer. Clients log in with `mnemon auth login --oidc` (authorization code + PKCE). See [Usage](USAGE.md).

**TODO (not in chart):** NetworkPolicy (Ingress/mesh only) and `readOnlyRootFilesystem: true` (add `emptyDir` at `/tmp`). ServiceMonitor, rate limit.

## Release

Tags `v*` publish CLI binaries, the server image, and the Helm chart. Pushes to `main` run CI and refresh `:dev`.

```bash
make release-snapshot
```
