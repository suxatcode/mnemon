#!/usr/bin/env bash
#
# Minikube integration suite for mnemon-server Helm installs and the mnemon CLI.
#
# Uses a dedicated minikube profile (default: mnemon-gateway) and never leaves
# kubectl's current context pointing at that profile.
#
# Usage:
#   bash scripts/minikube_gateway_test.sh
#   SCENARIOS=bundled,sqlite make test-minikube
#
set -euo pipefail

# Host Go is often 1.25+. Do not export GOTOOLCHAIN globally: it leaks into
# docker build and can rewrite go.mod.

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
RESET='\033[0m'

PASS=0
FAIL=0
TOTAL=0

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKDIR="${WORKDIR:-$ROOT/.testdata/minikube-gateway}"
CHART="$ROOT/deploy/helm/mnemon-server"
PROFILE="${MINIKUBE_PROFILE:-mnemon-gateway}"
NS="${MINIKUBE_NAMESPACE:-mnemon-gateway-test}"
RELEASE="${HELM_RELEASE:-mnemon}"
IMAGE_REPO="${IMAGE_REPO:-mnemon-dev/mnemon-server}"
IMAGE_TAG="${IMAGE_TAG:-dev}"
LOCAL_PORT="${LOCAL_PORT:-17443}"
SCENARIOS="${SCENARIOS:-bundled,external,rds,sqlite,tls-off}"
SKIP_BUILD="${SKIP_BUILD:-0}"
KEEP_CLUSTER="${KEEP_CLUSTER:-1}"
HELM_TIMEOUT="${HELM_TIMEOUT:-8m}"

MNEMON="$WORKDIR/mnemon"
ALICE_DIR="$WORKDIR/alice"
BOB_DIR="$WORKDIR/bob"
ORG_DIR="$WORKDIR/org"
CAROL_DIR="$WORKDIR/carol"
LOCAL_DIR="$WORKDIR/local-bypass"
INVITES="$WORKDIR/invites"
CA_FILE="$WORKDIR/ca.crt"
PF_PID=""
PREV_CTX=""
SCHEME="https"

banner() {
  echo ""
  echo -e "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
  echo -e "${BOLD}${CYAN}  $1${RESET}"
  echo -e "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
}

step() {
  echo ""
  echo -e "  ${YELLOW}▸${RESET} ${BOLD}$1${RESET}"
}

pass() {
  local label="$1"; local detail="${2:-}"
  TOTAL=$((TOTAL + 1)); PASS=$((PASS + 1))
  echo -e "    ${GREEN}✔${RESET} $label ${DIM}$detail${RESET}" >&2
}

fail() {
  local label="$1"; local detail="${2:-}"
  TOTAL=$((TOTAL + 1)); FAIL=$((FAIL + 1))
  echo -e "    ${RED}✘${RESET} $label ${DIM}$detail${RESET}" >&2
}

want_scenario() {
  [[ ",$SCENARIOS," == *",$1,"* ]]
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing required command: $1" >&2; exit 1; }
}

k() { kubectl --context "$PROFILE" -n "$NS" "$@"; }
h() { helm --kube-context "$PROFILE" -n "$NS" "$@"; }

cli() {
  local dir="$1"; shift
  MNEMON_DATA_DIR="$dir" "$MNEMON" "$@"
}

cli_capture() {
  local dir="$1"; shift
  local errf="$WORKDIR/last.err"
  local outf="$WORKDIR/last.out"
  local rc=0
  MNEMON_DATA_DIR="$dir" "$MNEMON" "$@" >"$outf" 2>"$errf" || rc=$?
  if [[ "$rc" -ne 0 ]]; then
    fail "$*" "exit $rc stderr=$(tr '\n' ' ' <"$errf") stdout=$(tr '\n' ' ' <"$outf")"
    echo "{}"
    return 0
  fi
  cat "$outf"
}

assert_contains() {
  if echo "$2" | grep -q "$3"; then
    pass "$1" "(contains: $3)"
  else
    fail "$1" "(expected: $3; got: $(echo "$2" | tr '\n' ' ' | head -c 240))"
  fi
}

assert_not_contains() {
  if echo "$2" | grep -q "$3"; then
    fail "$1" "(should NOT contain: $3)"
  else
    pass "$1" "(absent: $3)"
  fi
}

assert_jq() {
  local label="$1" json="$2" filter="$3" expected="$4"
  local actual
  actual=$(echo "$json" | jq -r "$filter" 2>/dev/null || echo "__ERROR__")
  if [ "$actual" = "$expected" ]; then
    pass "$label" "($filter == $expected)"
  else
    fail "$label" "($filter: expected=$expected, got=$actual)"
  fi
}

assert_exit_fails() {
  local label="$1"; shift
  local dir="$1"; shift
  local errf="$WORKDIR/last.err"
  if MNEMON_DATA_DIR="$dir" "$MNEMON" "$@" >"$WORKDIR/last.out" 2>"$errf"; then
    fail "$label" "command succeeded unexpectedly: $*"
  else
    pass "$label" "exit $(tr '\n' ' ' <"$errf" | head -c 160)"
  fi
}

dump_cluster() {
  echo ""
  echo -e "${YELLOW}── cluster dump ──${RESET}"
  k get pods,svc,pvc,secret,deploy,sts -o wide || true
  k get events --sort-by=.lastTimestamp | tail -n 40 || true
  k logs -l app.kubernetes.io/name=mnemon-server --tail=80 --all-containers=true || true
  k logs -l app.kubernetes.io/component=postgresql --tail=40 || true
}

stop_pf() {
  if [[ -n "${PF_PID:-}" ]] && kill -0 "$PF_PID" 2>/dev/null; then
    kill "$PF_PID" 2>/dev/null || true
    wait "$PF_PID" 2>/dev/null || true
  fi
  PF_PID=""
  pkill -f "kubectl --context $PROFILE -n $NS port-forward" 2>/dev/null || true
}

restore_ctx() {
  if [[ -n "${PREV_CTX:-}" ]]; then
    kubectl config use-context "$PREV_CTX" >/dev/null 2>&1 || true
  fi
}

cleanup_exit() {
  local code=$?
  stop_pf
  if [[ "$KEEP_CLUSTER" != "1" ]]; then
    helm --kube-context "$PROFILE" uninstall "$RELEASE" -n "$NS" >/dev/null 2>&1 || true
    kubectl --context "$PROFILE" delete ns "$NS" --wait=false >/dev/null 2>&1 || true
    minikube delete -p "$PROFILE" >/dev/null 2>&1 || true
  fi
  restore_ctx
  exit "$code"
}

trap cleanup_exit EXIT

start_minikube() {
  step "minikube profile $PROFILE"
  PREV_CTX="$(kubectl config current-context 2>/dev/null || true)"
  if ! minikube status -p "$PROFILE" >/dev/null 2>&1; then
    minikube start -p "$PROFILE" --driver=docker --cpus="${MINIKUBE_CPUS:-2}" --memory="${MINIKUBE_MEMORY:-3072}"
  fi
  restore_ctx
  kubectl --context "$PROFILE" get ns >/dev/null
  pass "minikube reachable" "context=$PROFILE (user context restored: ${PREV_CTX:-none})"
}

build_and_load() {
  step "build CLI and server image"
  mkdir -p "$WORKDIR" "$INVITES"
  if [[ "$SKIP_BUILD" != "1" ]]; then
    local gomod_bak="$WORKDIR/go.mod.bak" gosum_bak="$WORKDIR/go.sum.bak"
    cp "$ROOT/go.mod" "$gomod_bak"
    cp "$ROOT/go.sum" "$gosum_bak"
    # pgx v5.11 needs Go 1.25; keep go.mod pinned by restoring after the host build.
    GOTOOLCHAIN=local go build -mod=mod -o "$MNEMON" "$ROOT"
    cp "$gomod_bak" "$ROOT/go.mod"
    cp "$gosum_bak" "$ROOT/go.sum"
    env -u GOTOOLCHAIN docker build --target server \
      --build-arg VERSION="$IMAGE_TAG" \
      --build-arg GO_VERSION="${SERVER_GO_VERSION:-1.25.3}" \
      -t "${IMAGE_REPO}:${IMAGE_TAG}" "$ROOT"
  elif [[ ! -x "$MNEMON" ]]; then
    GOTOOLCHAIN=local go build -mod=mod -o "$MNEMON" "$ROOT"
    git -C "$ROOT" checkout -- go.mod go.sum >/dev/null 2>&1 || true
  fi
  minikube -p "$PROFILE" image load --overwrite=true "${IMAGE_REPO}:${IMAGE_TAG}"
  pass "image loaded" "${IMAGE_REPO}:${IMAGE_TAG}"
}

reset_namespace() {
  step "reset namespace $NS"
  stop_pf
  helm --kube-context "$PROFILE" uninstall "$RELEASE" -n "$NS" >/dev/null 2>&1 || true
  kubectl --context "$PROFILE" delete ns "$NS" --wait=true --timeout=120s >/dev/null 2>&1 || true
  kubectl --context "$PROFILE" create ns "$NS" >/dev/null
  rm -rf "$ALICE_DIR" "$BOB_DIR" "$ORG_DIR" "$CAROL_DIR" "$LOCAL_DIR" "$INVITES"
  mkdir -p "$ALICE_DIR" "$BOB_DIR" "$ORG_DIR" "$CAROL_DIR" "$LOCAL_DIR" "$INVITES"
}

install_chart() {
  local extra=("$@")
  step "helm upgrade --install $RELEASE (${extra[*]:-defaults})"
  local sets=(
    --set "image.repository=$IMAGE_REPO"
    --set "image.tag=$IMAGE_TAG"
    --set "image.pullPolicy=Never"
    --set "fullnameOverride=mnemon"
    --set "resources.requests.cpu=50m"
    --set "resources.requests.memory=128Mi"
    --set "resources.limits.memory=256Mi"
  )
  if ! h upgrade --install "$RELEASE" "$CHART" --wait --timeout "$HELM_TIMEOUT" \
      "${sets[@]}" "${extra[@]}"; then
    dump_cluster
    echo "helm install failed" >&2
    exit 1
  fi
  k rollout status deploy/mnemon --timeout=180s >/dev/null
}

start_pf() {
  stop_pf
  k port-forward "deploy/mnemon" "${LOCAL_PORT}:7443" >"$WORKDIR/pf.log" 2>&1 &
  PF_PID=$!
  local i
  for i in $(seq 1 60); do
    if curl -sk --max-time 1 "${SCHEME}://127.0.0.1:${LOCAL_PORT}/health" >/dev/null 2>&1; then
      pass "port-forward" "127.0.0.1:${LOCAL_PORT} ($SCHEME)"
      return 0
    fi
    sleep 0.5
  done
  fail "port-forward" "$(tr '\n' ' ' <"$WORKDIR/pf.log" | head -c 200)"
  dump_cluster
  return 1
}

curl_health() {
  local extra=()
  if [[ "$SCHEME" == "https" && -f "$CA_FILE" ]]; then
    extra=(--cacert "$CA_FILE")
  elif [[ "$SCHEME" == "https" ]]; then
    extra=(-k)
  fi
  curl -sS --max-time 5 "${extra[@]}" "${SCHEME}://127.0.0.1:${LOCAL_PORT}$1"
}

curl_code() {
  local extra=()
  local code
  if [[ "$SCHEME" == "https" && -f "$CA_FILE" ]]; then
    extra=(--cacert "$CA_FILE")
  elif [[ "$SCHEME" == "https" ]]; then
    extra=(-k)
  fi
  code=$(curl -sS --max-time 5 "${extra[@]}" -o "$WORKDIR/curl.body" -w '%{http_code}' "$@" || true)
  echo "${code:-000}"
}

save_ca() {
  if k get secret mnemon-app >/dev/null 2>&1 && k get secret mnemon-app -o json | jq -e '.data["ca.crt"]' >/dev/null 2>&1; then
    k get secret mnemon-app -o jsonpath='{.data.ca\.crt}' | base64 -d >"$CA_FILE"
  else
    rm -f "$CA_FILE"
  fi
}

issue_user() {
  local principal="$1" role="$2" outfile="$3"
  local server="127.0.0.1:${LOCAL_PORT}"
  local cmd=(mnemon-server user issue
    --principal "$principal" --role "$role"
    --server "$server"
    --jwt-key /config/jwt.key
    --data-dir /data
    --name team --out -)
  if [[ "$SCHEME" == "http" ]]; then
    cmd=(mnemon-server user issue
      --principal "$principal" --role "$role"
      --server "http://127.0.0.1:${LOCAL_PORT}"
      --jwt-key /config/jwt.key
      --data-dir /data
      --name team --out -)
  else
    cmd+=(--ca-file /config/ca.crt --server-name mnemon)
  fi
  k exec deploy/mnemon -- "${cmd[@]}" >"$outfile"
  jq -e '.token and .principal' "$outfile" >/dev/null
}

login_user() {
  local dir="$1" invite="$2"
  cli "$dir" auth login --default "$invite" >/dev/null
}

remember() {
  local dir="$1" content="$2"
  cli_capture "$dir" remember --no-diff "$content" --cat fact --imp 3
}

probe_k8s() {
  local expect_replicas="$1" expect_dialect_hint="$2"
  local ready
  ready=$(k get deploy mnemon -o jsonpath='{.status.readyReplicas}')
  if [[ "$ready" == "$expect_replicas" ]]; then
    pass "ready replicas" "$ready"
  else
    fail "ready replicas" "expected $expect_replicas got ${ready:-0}"
  fi
  local health readyj
  health=$(curl_health /health || true)
  readyj=$(curl_health /ready || true)
  assert_contains "GET /health" "$health" '"ok"'
  assert_contains "GET /ready" "$readyj" '"ready"'
  local unauth
  unauth=$(curl_code -X POST "${SCHEME}://127.0.0.1:${LOCAL_PORT}/v1/status")
  if [[ "$unauth" == "401" ]]; then
    pass "unauthenticated /v1/status" "401"
  else
    fail "unauthenticated /v1/status" "got $unauth body=$(tr '\n' ' ' <"$WORKDIR/curl.body")"
  fi
  if [[ "$expect_dialect_hint" == "postgres" ]]; then
    k get sts mnemon-postgresql >/dev/null 2>&1 && pass "bundled postgres sts" "present" || pass "postgres" "external/no bundled sts"
  elif [[ "$expect_dialect_hint" == "sqlite-pvc" ]]; then
    if k get pvc mnemon-data >/dev/null 2>&1; then
      pass "sqlite PVC" "mnemon-data"
    else
      fail "sqlite PVC" "missing"
    fi
    if k get sts mnemon-postgresql >/dev/null 2>&1; then
      fail "sqlite must not run bundled postgres" ""
    else
      pass "no bundled postgres" "sqlite mode"
    fi
  elif [[ "$expect_dialect_hint" == "sqlite" ]]; then
    if k get sts mnemon-postgresql >/dev/null 2>&1; then
      fail "sqlite must not run bundled postgres" ""
    else
      pass "no bundled postgres" "sqlite mode"
    fi
  fi
}

assert_tls_sans() {
  [[ "$SCHEME" == "https" ]] || return 0
  if ! command -v openssl >/dev/null 2>&1; then
    return 0
  fi
  local text
  text=$(echo | openssl s_client -connect "127.0.0.1:${LOCAL_PORT}" -servername mnemon 2>/dev/null | openssl x509 -noout -text 2>/dev/null || true)
  assert_contains "cert DNS SAN mnemon" "$text" "DNS:mnemon"
  assert_contains "cert DNS SAN localhost" "$text" "DNS:localhost"
  assert_contains "cert IP SAN 127.0.0.1" "$text" "IP Address:127.0.0.1"
  if [[ -f "$CA_FILE" ]]; then
    if curl -sS --cacert "$CA_FILE" --max-time 5 --resolve "mnemon:${LOCAL_PORT}:127.0.0.1" "https://mnemon:${LOCAL_PORT}/health" | grep -q ok; then
      pass "TLS verify via SNI mnemon" ""
    else
      fail "TLS verify via SNI mnemon" ""
    fi
    if curl -sS --cacert "$CA_FILE" --max-time 5 "https://127.0.0.1:${LOCAL_PORT}/health" | grep -q ok; then
      pass "TLS verify via IP SAN" ""
    else
      fail "TLS verify via IP SAN" ""
    fi
  fi
}

forged_jwt() {
  python3 - <<'PY'
import base64, hashlib, hmac, json, time
def b64(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode()
header = b64(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
payload = b64(json.dumps({
    "sub": "alice@team",
    "role": "user",
    "jti": "forged-jti",
    "exp": int(time.time()) + 3600,
}, separators=(",", ":")).encode())
sig = b64(hmac.new(b"wrong-key-wrong-key-wrong-key-xxxx", f"{header}.{payload}".encode(), hashlib.sha256).digest())
print(f"{header}.{payload}.{sig}")
PY
}

run_user_operator_flows() {
  local dialect="$1"
  banner "User / operator flows ($dialect)"

  step "issue alice, bob, org"
  issue_user "alice@team" user "$INVITES/alice.json"
  issue_user "bob@team" user "$INVITES/bob.json"
  issue_user "org@team" org "$INVITES/org.json"
  login_user "$ALICE_DIR" "$INVITES/alice.json"
  login_user "$BOB_DIR" "$INVITES/bob.json"
  login_user "$ORG_DIR" "$INVITES/org.json"
  pass "issued and logged in" "alice bob org"

  local alice_st
  alice_st=$(cli_capture "$ALICE_DIR" auth status)
  assert_jq "alice remote ok" "$alice_st" '.remote_status' 'ok'

  step "status redaction and dialect"
  local st
  st=$(cli_capture "$ALICE_DIR" status)
  assert_jq "status dialect" "$st" '.dialect' "$dialect"
  assert_jq "status remote ACL" "$st" '.remote' 'true'
  assert_jq "status principal" "$st" '.principal' 'alice@team'
  assert_jq "status max insights" "$st" '.max_insights' '25000'
  if [[ "$dialect" == "postgres" ]] && k get secret mnemon-postgresql >/dev/null 2>&1; then
    local pw
    pw=$(k get secret mnemon-postgresql -o jsonpath='{.data.postgres-password}' | base64 -d)
    assert_not_contains "status hides postgres password" "$st" "$pw"
  fi
  assert_not_contains "status has no postgres password scheme leak" "$st" ':s3cret@'
  if echo "$st" | grep -q 'postgres://'; then
    if echo "$st" | grep -Eq 'postgres://[^:]+:[^@]+@'; then
      local userinfo
      userinfo=$(echo "$st" | jq -r '.db_path' | sed -n 's#.*://\([^@]*\)@.*#\1#p')
      if echo "$userinfo" | grep -q ':xxxxx\|:redacted\|:\*\+\|xxxx'; then
        pass "postgres DSN password redacted" "$userinfo"
      elif echo "$userinfo" | grep -q ':'; then
        fail "postgres DSN still has a password" "$userinfo"
      else
        pass "postgres DSN has no password" "$userinfo"
      fi
    else
      pass "db_path does not embed userinfo password" "$(echo "$st" | jq -r '.db_path')"
    fi
  fi

  step "remember / recall / search isolation"
  local a1 a2 b1 o1
  a1=$(remember "$ALICE_DIR" "alice-only memory about widgets")
  a2=$(remember "$ALICE_DIR" "alice second note about gizmos")
  b1=$(remember "$BOB_DIR" "bob-only memory about sprockets")
  o1=$(remember "$ORG_DIR" "org-wide policy: use go 1.24")
  local aid bid oid aid2
  aid=$(echo "$a1" | jq -r '.id // empty' 2>/dev/null || true)
  aid2=$(echo "$a2" | jq -r '.id // empty' 2>/dev/null || true)
  bid=$(echo "$b1" | jq -r '.id // empty' 2>/dev/null || true)
  oid=$(echo "$o1" | jq -r '.id // empty' 2>/dev/null || true)
  if [[ -z "$aid" || "$aid" == "null" ]]; then
    fail "alice remember json" "$(echo "$a1" | tr '\n' ' ' | head -c 200)"
  fi
  assert_jq "alice owner" "$a1" '.owner_principal' 'alice@team'
  assert_jq "alice layer personal" "$a1" '.layer' 'personal'
  assert_jq "org owner" "$o1" '.owner_principal' 'org@team'
  assert_jq "org layer" "$o1" '.layer' 'org'

  local rec_a rec_b
  rec_a=$(cli_capture "$ALICE_DIR" recall "widgets" --limit 10)
  rec_b=$(cli_capture "$BOB_DIR" recall "widgets" --limit 10)
  assert_contains "alice recalls her widgets" "$rec_a" "alice-only memory about widgets"
  assert_contains "bob recalls alice widgets (team brain)" "$rec_b" "alice-only memory about widgets"
  local rec_org
  rec_org=$(cli_capture "$ALICE_DIR" recall "go 1.24" --limit 10)
  assert_contains "user recalls org layer" "$rec_org" "org-wide policy"
  assert_contains "org recall marks layer" "$rec_org" '"layer": "org"'

  local search_a
  search_a=$(cli_capture "$ALICE_DIR" search "sprockets" --limit 10)
  assert_contains "alice search finds bob" "$search_a" "bob-only memory about sprockets"

  step "ACL: forget / gc keep"
  assert_exit_fails "bob cannot forget alice" "$BOB_DIR" forget "$aid"
  assert_contains "bob forget error" "$(cat "$WORKDIR/last.err")" "forbidden"
  assert_exit_fails "alice cannot forget org" "$ALICE_DIR" forget "$oid"
  assert_contains "alice forget org error" "$(cat "$WORKDIR/last.err")" "forbidden"
  assert_exit_fails "bob cannot gc-keep alice" "$BOB_DIR" gc --keep "$aid"
  local forget_own
  forget_own=$(cli_capture "$ALICE_DIR" forget "$aid2")
  assert_jq "alice forgets own" "$forget_own" '.status' 'deleted'
  local rec_gone
  rec_gone=$(cli_capture "$BOB_DIR" recall "gizmos" --limit 10)
  assert_not_contains "forgotten insight hidden" "$rec_gone" "alice second note about gizmos"

  step "forged provenance tags"
  local forged
  forged=$(cli_capture "$ALICE_DIR" remember --no-diff "alice note with forged tags" --cat fact --imp 3 --tags "principal:bob@team,agent:evil")
  assert_contains "server provenance principal tag" "$forged" "principal:alice@team"
  assert_not_contains "forged principal tag stripped" "$forged" "principal:bob@team"
  assert_not_contains "forged agent tag stripped" "$forged" "agent:evil"
  assert_contains "cli agent tag applied" "$forged" "agent:mnemon-cli"

  step "client cannot forge owner via extra JSON"
  local token
  token=$(jq -r '.token' "$INVITES/alice.json")
  local curl_extra=()
  if [[ "$SCHEME" == "https" && -f "$CA_FILE" ]]; then
    curl_extra=(--cacert "$CA_FILE")
  fi
  curl -sS --max-time 10 "${curl_extra[@]}" \
    -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
    -d '{"content":"forged-owner-body","category":"fact","importance":3,"owner_principal":"bob@team","layer":"org","no_diff":true}' \
    "${SCHEME}://127.0.0.1:${LOCAL_PORT}/v1/remember" >"$WORKDIR/forged-owner.json"
  assert_jq "API ignores forged owner" "$(jq -c '.result' "$WORKDIR/forged-owner.json")" '.owner_principal' 'alice@team'
  assert_jq "API ignores forged layer" "$(jq -c '.result' "$WORKDIR/forged-owner.json")" '.layer' 'personal'

  step "forged JWT"
  local bad
  bad=$(curl_code -H "Authorization: Bearer $(forged_jwt)" -X POST "${SCHEME}://127.0.0.1:${LOCAL_PORT}/v1/status")
  if [[ "$bad" == "401" ]]; then
    pass "forged JWT rejected" "401"
  else
    fail "forged JWT rejected" "got $bad"
  fi

  step "link / related / log / gc / receipt / viz / import / embed --status"
  local a3
  a3=$(remember "$ALICE_DIR" "alice related note about widgets graph")
  local aid3
  aid3=$(echo "$a3" | jq -r '.id // empty' 2>/dev/null || true)
  local linked
  linked=$(cli_capture "$ALICE_DIR" link "$aid" "$aid3" --type semantic --weight 0.8)
  assert_jq "alice link" "$linked" '.status' 'linked'
  local rel
  rel=$(cli_capture "$BOB_DIR" related "$aid" --edge semantic)
  assert_contains "bob related sees alice graph" "$rel" "$aid3"
  local logj
  logj=$(cli_capture "$ALICE_DIR" log --limit 5)
  assert_contains "oplog present" "$logj" "operation"
  local gcj
  gcj=$(cli_capture "$ALICE_DIR" gc --threshold 0.1 --limit 20)
  assert_contains "gc suggest" "$gcj" "candidates"
  local recpt
  recpt=$(cli_capture "$ALICE_DIR" receipt --limit 5)
  assert_jq "receipt schema" "$recpt" '.schema' 'mnemon.memory.receipt.v1'
  local viz
  viz=$(cli_capture "$ALICE_DIR" viz --format dot)
  assert_contains "viz dot" "$viz" "digraph"
  cat >"$WORKDIR/draft.json" <<'EOF'
{
  "schema_version": "1",
  "insights": [
    {
      "content": "imported alice draft about widgets",
      "category": "fact",
      "importance": 3,
      "tags": ["import"],
      "entities": ["WidgetCo"]
    }
  ]
}
EOF
  local dry imp
  dry=$(cli_capture "$ALICE_DIR" import --dry-run "$WORKDIR/draft.json")
  assert_jq "import dry-run" "$dry" '.status' 'dry_run_ok'
  imp=$(cli_capture "$ALICE_DIR" import --no-diff "$WORKDIR/draft.json")
  assert_contains "import wrote" "$imp" "imported alice draft about widgets"
  local emb
  emb=$(cli_capture "$ALICE_DIR" embed --status)
  assert_contains "embed status" "$emb" "total_insights"

  step "hot issue carol (no restart)"
  local uids_before uids_after
  uids_before=$(k get pod -l app.kubernetes.io/name=mnemon-server -o jsonpath='{.items[*].metadata.uid}' | tr ' ' '\n' | sort | tr '\n' ' ')
  issue_user "carol@team" user "$INVITES/carol.json"
  login_user "$CAROL_DIR" "$INVITES/carol.json"
  local c1
  c1=$(remember "$CAROL_DIR" "carol joined without restart")
  assert_jq "carol remember" "$c1" '.owner_principal' 'carol@team'
  uids_after=$(k get pod -l app.kubernetes.io/name=mnemon-server -o jsonpath='{.items[*].metadata.uid}' | tr ' ' '\n' | sort | tr '\n' ' ')
  if [[ "$uids_before" == "$uids_after" ]]; then
    pass "issue did not roll pods" "uids unchanged"
  else
    fail "issue rolled pods" "$uids_before -> $uids_after"
  fi

  step "revoke bob"
  k exec deploy/mnemon -- mnemon-server user revoke --principal bob@team --data-dir /data >/dev/null
  assert_exit_fails "revoked bob cannot status" "$BOB_DIR" status
  local still
  still=$(cli_capture "$ALICE_DIR" status)
  assert_jq "alice still works after bob revoke" "$still" '.principal' 'alice@team'

  step "--local bypass"
  local loc
  loc=$(cli_capture "$ALICE_DIR" --local remember --no-diff "this stays on the laptop" --cat fact --imp 3)
  assert_jq "local remember not remote owner" "$loc" '.owner_principal' 'local'
  local loc_st
  loc_st=$(cli_capture "$ALICE_DIR" --local status)
  assert_jq "local status sqlite" "$loc_st" '.dialect' 'sqlite'
  assert_jq "local status not remote" "$loc_st" '.remote' 'false'
  local remote_rec
  remote_rec=$(cli_capture "$ALICE_DIR" recall "this stays on the laptop" --limit 10)
  assert_not_contains "local note not on gateway" "$remote_rec" "this stays on the laptop"
}

deploy_external_postgres() {
  step "external postgres + mnemon-db secret"
  k apply -f - <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: ext-pg
type: Opaque
stringData:
  postgres-password: testdbpass
---
apiVersion: v1
kind: Service
metadata:
  name: ext-pg
spec:
  ports:
    - port: 5432
      targetPort: 5432
  selector:
    app: ext-pg
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ext-pg
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ext-pg
  template:
    metadata:
      labels:
        app: ext-pg
    spec:
      containers:
        - name: postgres
          image: postgres:16-alpine
          ports:
            - containerPort: 5432
          env:
            - name: POSTGRES_USER
              value: mnemon
            - name: POSTGRES_DB
              value: mnemon
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: ext-pg
                  key: postgres-password
          readinessProbe:
            exec:
              command: ["pg_isready", "-U", "mnemon"]
            initialDelaySeconds: 3
            periodSeconds: 5
EOF
  k rollout status deploy/ext-pg --timeout=180s >/dev/null
  k delete secret mnemon-db --ignore-not-found >/dev/null
  k create secret generic mnemon-db \
    --from-literal=url='postgres://mnemon:testdbpass@ext-pg:5432/mnemon?sslmode=disable' >/dev/null
  pass "external postgres ready" "ext-pg"
}

smoke_client() {
  local dialect="$1" note="$2"
  issue_user "alice@team" user "$INVITES/alice.json"
  login_user "$ALICE_DIR" "$INVITES/alice.json"
  local st rec
  st=$(cli_capture "$ALICE_DIR" status)
  assert_jq "smoke dialect ($note)" "$st" '.dialect' "$dialect"
  rec=$(remember "$ALICE_DIR" "smoke $note")
  assert_jq "smoke remember ($note)" "$rec" '.owner_principal' 'alice@team'
  local reca
  reca=$(cli_capture "$ALICE_DIR" recall "smoke $note" --limit 5)
  assert_contains "smoke recall ($note)" "$reca" "smoke $note"
}

scenario_bundled() {
  banner "A. Bundled Postgres (replicas=2, TLS)"
  SCHEME=https
  reset_namespace
  install_chart --set replicaCount=2
  save_ca
  start_pf
  probe_k8s 2 postgres
  assert_tls_sans
  run_user_operator_flows postgres
  stop_pf
}

scenario_external() {
  banner "B. External Postgres DSN (existingSecret)"
  SCHEME=https
  reset_namespace
  deploy_external_postgres
  install_chart --set postgresql.enabled=false --set database.existingSecret=mnemon-db --set replicaCount=2
  save_ca
  start_pf
  probe_k8s 2 postgres
  smoke_client postgres "external-dsn"
  local st
  st=$(cli_capture "$ALICE_DIR" status)
  assert_not_contains "external DSN password redacted" "$st" "testdbpass"
  stop_pf
}

scenario_rds() {
  banner "C. values-rds.yaml overlay"
  SCHEME=https
  if ! k get deploy ext-pg >/dev/null 2>&1; then
    reset_namespace
    deploy_external_postgres
  fi
  if ! k get secret mnemon-db >/dev/null 2>&1; then
    k create secret generic mnemon-db \
      --from-literal=url='postgres://mnemon:testdbpass@ext-pg:5432/mnemon?sslmode=disable' >/dev/null
  fi
  install_chart -f "$CHART/values-rds.yaml" \
    --set image.pullPolicy=Never \
    --set image.repository="$IMAGE_REPO" \
    --set image.tag="$IMAGE_TAG" \
    --set fullnameOverride=mnemon \
    --set resources.requests.cpu=50m \
    --set resources.requests.memory=128Mi \
    --set resources.limits.memory=256Mi
  save_ca
  start_pf
  probe_k8s 2 postgres
  smoke_client postgres "rds-overlay"
  if k get sts mnemon-postgresql >/dev/null 2>&1; then
    fail "rds overlay must not deploy bundled postgres" ""
  else
    pass "rds overlay has no bundled postgres" ""
  fi
  stop_pf
}

scenario_sqlite() {
  banner "D. SQLite PVC + restart"
  SCHEME=https
  reset_namespace
  install_chart --set postgresql.enabled=false --set persistence.enabled=true --set replicaCount=2
  save_ca
  start_pf
  probe_k8s 1 sqlite-pvc
  issue_user "alice@team" user "$INVITES/alice.json"
  login_user "$ALICE_DIR" "$INVITES/alice.json"
  local rec st
  rec=$(remember "$ALICE_DIR" "sqlite survives restart")
  st=$(cli_capture "$ALICE_DIR" status)
  assert_jq "sqlite dialect" "$st" '.dialect' 'sqlite'
  stop_pf
  k delete pod -l app.kubernetes.io/name=mnemon-server --wait=true --timeout=120s >/dev/null
  k rollout status deploy/mnemon --timeout=180s >/dev/null
  start_pf
  local reca
  reca=$(cli_capture "$ALICE_DIR" recall "sqlite survives restart" --limit 5)
  assert_contains "memory survived pod delete" "$reca" "sqlite survives restart"
  stop_pf
}

scenario_tls_off() {
  banner "E. TLS disabled (HTTP)"
  SCHEME=http
  reset_namespace
  install_chart --set server.tls.enabled=false --set postgresql.enabled=false --set persistence.enabled=false
  rm -f "$CA_FILE"
  start_pf
  probe_k8s 1 sqlite
  smoke_client sqlite "tls-off"
  python3 - "$ALICE_DIR" <<'PY'
import json, sys
from pathlib import Path
p = Path(sys.argv[1]) / "auth.json"
cfg = json.loads(p.read_text())
for remote in cfg.get("remotes", []):
    server = remote.get("server", "")
    if server.startswith("http://"):
        remote["server"] = "https://" + server[len("http://"):]
p.write_text(json.dumps(cfg, indent=2) + "\n")
PY
  assert_exit_fails "HTTPS client against HTTP server" "$ALICE_DIR" status
  stop_pf
}

summary() {
  banner "Summary"
  echo -e "    ${GREEN}$PASS passed${RESET}  ${RED}$FAIL failed${RESET}  $TOTAL total"
  if [[ "$FAIL" -gt 0 ]]; then
    dump_cluster
    exit 1
  fi
}

main() {
  banner "Mnemon minikube gateway suite"
  echo -e "  profile=${BOLD}$PROFILE${RESET}  ns=$NS  scenarios=$SCENARIOS"
  require_cmd minikube
  require_cmd kubectl
  require_cmd helm
  require_cmd docker
  require_cmd jq
  require_cmd python3
  require_cmd curl
  require_cmd go
  mkdir -p "$WORKDIR"
  start_minikube
  build_and_load
  want_scenario bundled && scenario_bundled
  want_scenario external && scenario_external
  want_scenario rds && scenario_rds
  want_scenario sqlite && scenario_sqlite
  want_scenario tls-off && scenario_tls_off
  summary
}

main "$@"
