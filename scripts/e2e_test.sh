#!/usr/bin/env bash
#
# Mnemon E2E visual test
# Usage: ./scripts/e2e_test.sh
#
set -euo pipefail

# ── Colors ────────────────────────────────────────────────────────────
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

# ── Helpers ───────────────────────────────────────────────────────────
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

show_json() {
  echo "$1" | jq '.' 2>/dev/null | head -"${2:-20}" | sed 's/^/    /'
}

pass() {
  local label="$1"; local detail="$2"
  TOTAL=$((TOTAL + 1)); PASS=$((PASS + 1))
  echo -e "    ${GREEN}✔${RESET} $label ${DIM}$detail${RESET}"
}

fail() {
  local label="$1"; local detail="$2"
  TOTAL=$((TOTAL + 1)); FAIL=$((FAIL + 1))
  echo -e "    ${RED}✘${RESET} $label ${DIM}$detail${RESET}"
}

# assert_contains LABEL JSON NEEDLE
assert_contains() {
  if echo "$2" | grep -q "$3"; then
    pass "$1" "(contains: $3)"
  else
    fail "$1" "(expected: $3)"
  fi
}

# assert_not_contains LABEL JSON NEEDLE
assert_not_contains() {
  if echo "$2" | grep -q "$3"; then
    fail "$1" "(should NOT contain: $3)"
  else
    pass "$1" "(absent: $3)"
  fi
}

# assert_jq LABEL JSON JQ_FILTER EXPECTED
# e.g. assert_jq "total is 1" "$OUT" '.total_insights' '1'
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

# assert_jq_gte LABEL JSON JQ_FILTER EXPECTED
assert_jq_gte() {
  local label="$1" json="$2" filter="$3" expected="$4"
  local actual
  actual=$(echo "$json" | jq -r "$filter" 2>/dev/null || echo "0")
  if [ "$actual" -ge "$expected" ] 2>/dev/null; then
    pass "$label" "($filter=$actual >= $expected)"
  else
    fail "$label" "($filter: expected >= $expected, got=$actual)"
  fi
}

extract_id() {
  echo "$1" | jq -r '.id'
}

# ── Setup ─────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
TESTDATA="$PROJECT_DIR/.testdata"
TESTDIR="$TESTDATA/m1"
M="$PROJECT_DIR/mnemon"

banner "Building mnemon"
cd "$PROJECT_DIR"
go build -o mnemon .
echo -e "  ${GREEN}✔${RESET} Binary built: $M"

# Clean previous test data
rm -rf "$TESTDATA"
mkdir -p "$TESTDIR"
echo -e "  ${DIM}  Test data: $TESTDATA/${RESET}"

# ══════════════════════════════════════════════════════════════════════
banner "Milestone 1: Basic CRUD"
# ══════════════════════════════════════════════════════════════════════

step "remember — store insight with tags"
OUT=$($M --data-dir "$TESTDIR" remember "User prefers Qdrant for vector DB" --cat preference --imp 4 --tags "tool,db")
show_json "$OUT" 20
ID1=$(extract_id "$OUT")
assert_jq "category is preference" "$OUT" '.category' 'preference'
assert_jq "importance is 4"        "$OUT" '.importance' '4'
assert_contains "tags include tool" "$OUT" '"tool"'
assert_contains "entities is []"    "$OUT" '"entities": \[\]'

step "recall — keyword search"
OUT=$($M --data-dir "$TESTDIR" recall "Qdrant")
show_json "$OUT" 10
assert_contains "found Qdrant insight" "$OUT" "User prefers Qdrant"

step "recall — no match returns []"
OUT=$($M --data-dir "$TESTDIR" recall "nonexistent_xyz")
assert_jq "empty array" "$OUT" 'length' '0'

step "status — statistics"
OUT=$($M --data-dir "$TESTDIR" status)
show_json "$OUT"
assert_jq "total is 1"          "$OUT" '.total_insights' '1'
assert_jq "preference count"    "$OUT" '.by_category.preference' '1'
assert_jq "no deleted insights" "$OUT" '.deleted_insights' '0'

step "forget — soft delete"
OUT=$($M --data-dir "$TESTDIR" forget "$ID1")
show_json "$OUT"
assert_jq "status is deleted" "$OUT" '.status' 'deleted'

OUT=$($M --data-dir "$TESTDIR" status)
assert_jq "total now 0"    "$OUT" '.total_insights'   '0'
assert_jq "deleted now 1"  "$OUT" '.deleted_insights'  '1'

# ══════════════════════════════════════════════════════════════════════
banner "Milestone 2: Graph Edge Auto-Generation"
# ══════════════════════════════════════════════════════════════════════

# Fresh DB for cleaner edge tests
TESTDIR2="$TESTDATA/m2"
mkdir -p "$TESTDIR2"

step "remember 3 insights — check temporal + causal edges"
OUT1=$($M --data-dir "$TESTDIR2" remember "User prefers Qdrant for vector DB" --cat preference --imp 4)
ID_A=$(extract_id "$OUT1")
assert_jq "first: no temporal" "$OUT1" '.edges_created.temporal' '0'

sleep 1  # ensure distinct timestamps

OUT2=$($M --data-dir "$TESTDIR2" remember "Chose Qdrant because of Rust performance" --cat decision --imp 5)
ID_B=$(extract_id "$OUT2")
assert_jq "second: 2 temporal" "$OUT2" '.edges_created.temporal' '2'
assert_jq "second: 1 causal"   "$OUT2" '.edges_created.causal'   '1'

sleep 1

OUT3=$($M --data-dir "$TESTDIR2" remember "Qdrant benchmark shows 10ms p99 latency" --cat fact --imp 3)
ID_C=$(extract_id "$OUT3")
assert_jq "third: 2 temporal" "$OUT3" '.edges_created.temporal' '2'

step "status — verify edge count"
OUT=$($M --data-dir "$TESTDIR2" status)
assert_jq_gte "edges >= 5" "$OUT" '.edge_count' '5'

step "related — temporal traversal from B"
OUT=$($M --data-dir "$TESTDIR2" related "$ID_B" --edge temporal)
show_json "$OUT" 20
assert_contains "finds A via temporal" "$OUT" "$ID_A"
assert_contains "finds C via temporal" "$OUT" "$ID_C"

step "related — causal traversal from B"
OUT=$($M --data-dir "$TESTDIR2" related "$ID_B" --edge causal)
show_json "$OUT" 10
assert_contains "finds A via causal" "$OUT" "$ID_A"

step "Entity extraction — CamelCase"
OUT=$($M --data-dir "$TESTDIR2" remember "We use HttpServer and DataStore in the project" --cat fact)
echo -e "    ${DIM}entities: $(echo "$OUT" | jq -c '.entities')${RESET}"
assert_contains "HttpServer extracted" "$OUT" '"HttpServer"'
assert_contains "DataStore extracted"  "$OUT" '"DataStore"'
ID_D=$(extract_id "$OUT")

sleep 1

step "Entity edge — shared entity creates link"
OUT=$($M --data-dir "$TESTDIR2" remember "HttpServer handles all API requests" --cat fact)
echo -e "    ${DIM}entities: $(echo "$OUT" | jq -c '.entities')  edges: $(echo "$OUT" | jq -c '.edges_created')${RESET}"
assert_jq_gte "entity edges created (bidirectional)" "$OUT" '.edges_created.entity' '2'
ID_E=$(extract_id "$OUT")

step "Entity edge — bidirectional traversal"
OUT=$($M --data-dir "$TESTDIR2" related "$ID_E" --edge entity)
assert_contains "E → D via entity" "$OUT" "$ID_D"
OUT=$($M --data-dir "$TESTDIR2" related "$ID_D" --edge entity)
assert_contains "D → E via entity (reverse)" "$OUT" "$ID_E"

step "Entity extraction — file paths"
OUT=$($M --data-dir "$TESTDIR2" remember "Config lives at ./cmd/root.go and internal/store/db.go" --cat fact)
echo -e "    ${DIM}entities: $(echo "$OUT" | jq -c '.entities')${RESET}"
assert_contains "file path extracted" "$OUT" './cmd/root.go'

step "Entity extraction — Chinese book titles"
OUT=$($M --data-dir "$TESTDIR2" remember "推荐阅读《深入理解计算机系统》这本书" --cat fact)
echo -e "    ${DIM}entities: $(echo "$OUT" | jq -c '.entities')${RESET}"
assert_contains "Chinese title extracted" "$OUT" '深入理解计算机系统'

# ══════════════════════════════════════════════════════════════════════
banner "Milestone 3: Search + Diff"
# ══════════════════════════════════════════════════════════════════════

step "search — token-scored search"
OUT=$($M --data-dir "$TESTDIR2" search "Rust performance")
show_json "$OUT" 15
assert_contains "finds decision insight" "$OUT" "Chose Qdrant"
assert_contains "has score field"        "$OUT" '"score"'

step "search — no match returns []"
OUT=$($M --data-dir "$TESTDIR2" search "zzz_no_match_zzz")
assert_jq "empty array" "$OUT" 'length' '0'

step "diff — DUPLICATE detection"
OUT=$($M --data-dir "$TESTDIR2" diff "User prefers Qdrant for vector DB")
echo -e "    ${DIM}suggestion: $(echo "$OUT" | jq -r '.suggestion')${RESET}"
assert_jq "suggestion is DUPLICATE" "$OUT" '.suggestion' 'DUPLICATE'

step "diff — CONFLICT detection (negation)"
OUT=$($M --data-dir "$TESTDIR2" diff "User no longer prefers Qdrant for vector DB")
echo -e "    ${DIM}suggestion: $(echo "$OUT" | jq -r '.suggestion')${RESET}"
assert_jq "suggestion is CONFLICT" "$OUT" '.suggestion' 'CONFLICT'

step "diff — ADD for unrelated"
OUT=$($M --data-dir "$TESTDIR2" diff "Redis is great for caching")
echo -e "    ${DIM}suggestion: $(echo "$OUT" | jq -r '.suggestion')${RESET}"
assert_jq "suggestion is ADD" "$OUT" '.suggestion' 'ADD'

# ══════════════════════════════════════════════════════════════════════
banner "Milestone 4: Intent-Aware Smart Recall"
# ══════════════════════════════════════════════════════════════════════

# Fresh DB for multi-level traversal test
TESTDIR3="$TESTDATA/m4"
mkdir -p "$TESTDIR3"

step "multi-level traversal — build A→B→C causal chain"
# A: anchor (keyword-matched), B: 1-hop, C: 2-hop (only reachable with depth>=2)
OUT_X=$($M --data-dir "$TESTDIR3" remember "Alpha service handles request routing" --cat fact --imp 3)
ID_X=$(extract_id "$OUT_X")

sleep 1

OUT_Y=$($M --data-dir "$TESTDIR3" remember "Request routing uses Alpha service because of low latency" --cat decision --imp 4)
ID_Y=$(extract_id "$OUT_Y")
assert_jq_gte "Y has causal edge to X" "$OUT_Y" '.edges_created.causal' '1'

sleep 1

OUT_Z=$($M --data-dir "$TESTDIR3" remember "Low latency achieved because of edge caching" --cat fact --imp 3)
ID_Z=$(extract_id "$OUT_Z")
assert_jq_gte "Z has causal edge to Y" "$OUT_Z" '.edges_created.causal' '1'

step "multi-level traversal — smart recall finds depth-2 node"
OUT=$($M --data-dir "$TESTDIR3" recall "why Alpha service routing" --smart)
echo -e "    ${DIM}intent: $(echo "$OUT" | jq -r '.[0].intent // "N/A"')  results: $(echo "$OUT" | jq 'length')${RESET}"
echo "$OUT" | jq -r '.[] | "    \(.via)\t\(.score | tostring | .[:6])\t\(.insight.content[:50])"' 2>/dev/null
assert_contains "finds anchor X (Alpha)" "$OUT" "$ID_X"
assert_contains "finds depth-1 Y (routing)" "$OUT" "$ID_Y"
assert_contains "finds depth-2 Z (caching)" "$OUT" "$ID_Z"

step "smart recall — WHY intent"
OUT=$($M --data-dir "$TESTDIR2" recall "why did we choose Qdrant" --smart)
echo -e "    ${DIM}intent: $(echo "$OUT" | jq -r '.[0].intent // "N/A"')  results: $(echo "$OUT" | jq 'length')${RESET}"
assert_contains "intent is WHY" "$OUT" '"WHY"'
assert_contains "finds Qdrant insight" "$OUT" "Qdrant"

step "smart recall — WHEN intent"
OUT=$($M --data-dir "$TESTDIR2" recall "when did we choose vector db" --smart)
echo -e "    ${DIM}intent: $(echo "$OUT" | jq -r '.[0].intent // "N/A"')  results: $(echo "$OUT" | jq 'length')${RESET}"
assert_contains "intent is WHEN" "$OUT" '"WHEN"'

step "smart recall — graph augments results"
OUT=$($M --data-dir "$TESTDIR2" recall "why Qdrant performance" --smart)
COUNT=$(echo "$OUT" | jq 'length')
TOTAL=$((TOTAL + 1))
if [ "$COUNT" -ge 2 ]; then
  PASS=$((PASS + 1))
  echo -e "    ${GREEN}✔${RESET} Returns multiple results ${DIM}(count=$COUNT)${RESET}"
else
  FAIL=$((FAIL + 1))
  echo -e "    ${RED}✘${RESET} Expected >= 2 results, got $COUNT"
fi

# ══════════════════════════════════════════════════════════════════════
banner "Observability: Operation Log"
# ══════════════════════════════════════════════════════════════════════

step "log — shows operations from TESTDIR2"
OUT=$($M --data-dir "$TESTDIR2" log --limit 30)
echo "$OUT" | head -10 | sed 's/^/    /'
assert_contains "log has remember ops" "$OUT" "remember"
assert_contains "log has recall ops"   "$OUT" "recall"

# ── Report ────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo -e "${BOLD}${CYAN}  Results${RESET}"
echo -e "${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"
echo ""
echo -e "  Total:  ${BOLD}$TOTAL${RESET}"
echo -e "  Passed: ${GREEN}${BOLD}$PASS${RESET}"
if [ "$FAIL" -gt 0 ]; then
  echo -e "  Failed: ${RED}${BOLD}$FAIL${RESET}"
fi
echo ""

# Cleanup binary (keep .testdata for inspection)
rm -f "$M"
echo -e "  ${DIM}Test DBs preserved at: $TESTDATA/${RESET}"
echo -e "  ${DIM}Run 'rm -rf .testdata' to clean up${RESET}"

if [ "$FAIL" -gt 0 ]; then
  echo -e "  ${RED}${BOLD}FAIL${RESET}"
  exit 1
else
  echo -e "  ${GREEN}${BOLD}ALL PASSED ✔${RESET}"
  exit 0
fi
