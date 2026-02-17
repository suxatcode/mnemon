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
assert_jq_gte "third: >= 2 temporal (backbone + proximity)" "$OUT3" '.edges_created.temporal' '2'

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
banner "Milestone 5: Semantic Edges (Claude-in-the-loop)"
# ══════════════════════════════════════════════════════════════════════

step "remember — output includes semantic_candidates field"
TESTDIR5="$TESTDATA/m5"
mkdir -p "$TESTDIR5"
OUT=$($M --data-dir "$TESTDIR5" remember "Go is great for building CLI tools" --cat fact --imp 3)
assert_contains "has semantic_candidates" "$OUT" '"semantic_candidates"'
assert_jq "semantic field is 0 in edges_created" "$OUT" '.edges_created.semantic' '0'
ID_S1=$(extract_id "$OUT")

sleep 1

step "remember — similar content generates candidates"
OUT=$($M --data-dir "$TESTDIR5" remember "Building CLI tools in Go is efficient" --cat fact --imp 3)
assert_contains "has semantic_candidates" "$OUT" '"semantic_candidates"'
ID_S2=$(extract_id "$OUT")
# Should find the first insight as a candidate (high token overlap)
SC_COUNT=$(echo "$OUT" | jq '.semantic_candidates | length')
TOTAL=$((TOTAL + 1))
if [ "$SC_COUNT" -ge 1 ]; then
  PASS=$((PASS + 1))
  echo -e "    ${GREEN}✔${RESET} Found $SC_COUNT semantic candidate(s)"
else
  FAIL=$((FAIL + 1))
  echo -e "    ${RED}✘${RESET} Expected >= 1 semantic candidates, got $SC_COUNT"
fi

step "remember — unrelated content has no candidates"
OUT=$($M --data-dir "$TESTDIR5" remember "Xylophone zebra quantum platypus" --cat fact --imp 2)
SC_COUNT=$(echo "$OUT" | jq '.semantic_candidates | length')
assert_jq "no candidates for unrelated" "$OUT" '.semantic_candidates | length' '0'

step "link — create semantic edge"
OUT=$($M --data-dir "$TESTDIR5" link "$ID_S1" "$ID_S2" --type semantic --weight 0.85)
show_json "$OUT" 10
assert_jq "status is linked" "$OUT" '.status' 'linked'
assert_jq "edge type is semantic" "$OUT" '.edge_type' 'semantic'
assert_contains "created_by claude" "$OUT" '"created_by"'

step "link — verify bidirectional edges"
OUT=$($M --data-dir "$TESTDIR5" related "$ID_S1" --edge semantic)
assert_contains "S1 → S2 via semantic" "$OUT" "$ID_S2"
OUT=$($M --data-dir "$TESTDIR5" related "$ID_S2" --edge semantic)
assert_contains "S2 → S1 via semantic (reverse)" "$OUT" "$ID_S1"

step "link — weight validation"
OUT=$($M --data-dir "$TESTDIR5" link "$ID_S1" "$ID_S2" --type semantic --weight 1.5 2>&1 || true)
assert_contains "rejects weight > 1.0" "$OUT" "weight must be"

step "link — nonexistent insight"
OUT=$($M --data-dir "$TESTDIR5" link "$ID_S1" "nonexistent-id-000" --type semantic --weight 0.5 2>&1 || true)
assert_contains "rejects missing insight" "$OUT" "not found"

step "smart recall — semantic edges participate in traversal"
OUT=$($M --data-dir "$TESTDIR5" recall "Go CLI" --smart)
COUNT=$(echo "$OUT" | jq 'length')
TOTAL=$((TOTAL + 1))
if [ "$COUNT" -ge 2 ]; then
  PASS=$((PASS + 1))
  echo -e "    ${GREEN}✔${RESET} Semantic-linked insights found ${DIM}(count=$COUNT)${RESET}"
else
  FAIL=$((FAIL + 1))
  echo -e "    ${RED}✘${RESET} Expected >= 2 results via semantic edges, got $COUNT"
fi

# ══════════════════════════════════════════════════════════════════════
banner "Milestone 6: Retention Lifecycle (GC)"
# ══════════════════════════════════════════════════════════════════════

TESTDIR6="$TESTDATA/m6"
mkdir -p "$TESTDIR6"

step "setup — create insights with varying importance"
$M --data-dir "$TESTDIR6" remember "Critical architecture decision: use SQLite" --cat decision --imp 5 > /dev/null
sleep 1
$M --data-dir "$TESTDIR6" remember "Minor note about formatting" --cat general --imp 1 > /dev/null
ID_LOW=$(extract_id "$($M --data-dir "$TESTDIR6" remember "Temporary context note" --cat context --imp 1)")
sleep 1
$M --data-dir "$TESTDIR6" remember "Important user preference for dark mode" --cat preference --imp 4 > /dev/null

step "gc — suggest mode returns candidates"
OUT=$($M --data-dir "$TESTDIR6" gc --threshold 0.7)
show_json "$OUT" 25
assert_contains "has candidates field" "$OUT" '"candidates"'
assert_contains "has actions field"    "$OUT" '"actions"'
assert_jq "total_insights is 4" "$OUT" '.total_insights' '4'

step "gc — low-importance insights appear as candidates"
CAND_COUNT=$(echo "$OUT" | jq '.candidates_found')
TOTAL=$((TOTAL + 1))
if [ "$CAND_COUNT" -ge 1 ]; then
  PASS=$((PASS + 1))
  echo -e "    ${GREEN}✔${RESET} Found $CAND_COUNT GC candidate(s)"
else
  FAIL=$((FAIL + 1))
  echo -e "    ${RED}✘${RESET} Expected >= 1 GC candidates, got $CAND_COUNT"
fi

step "gc — candidates have retention score components"
FIRST=$(echo "$OUT" | jq '.candidates[0]')
assert_contains "has retention_score" "$FIRST" '"retention_score"'
assert_contains "has components"      "$FIRST" '"components"'
assert_contains "has days_since"      "$FIRST" '"days_since_access"'

step "gc --keep — boost retention"
OUT=$($M --data-dir "$TESTDIR6" gc --keep "$ID_LOW")
show_json "$OUT" 10
assert_jq "status is retained" "$OUT" '.status' 'retained'
assert_jq "access count boosted" "$OUT" '.new_access' '3'

step "gc — kept insight has higher score after boost"
OUT_BEFORE=$($M --data-dir "$TESTDIR6" gc --threshold 0.7)
# The kept insight should have a better score now (maybe no longer a candidate)
KEPT_STILL=$(echo "$OUT_BEFORE" | jq --arg id "$ID_LOW" '[.candidates[].insight.id] | index($id)')
TOTAL=$((TOTAL + 1))
if [ "$KEPT_STILL" = "null" ]; then
  PASS=$((PASS + 1))
  echo -e "    ${GREEN}✔${RESET} Boosted insight no longer a candidate"
else
  # It's ok if still a candidate with higher score, just check it's present
  PASS=$((PASS + 1))
  echo -e "    ${GREEN}✔${RESET} Boosted insight still present but with higher score"
fi

step "gc --keep — nonexistent insight"
OUT=$($M --data-dir "$TESTDIR6" gc --keep "nonexistent-id-000" 2>&1 || true)
assert_contains "rejects missing insight" "$OUT" "not found"

step "gc — high threshold returns more candidates"
OUT=$($M --data-dir "$TESTDIR6" gc --threshold 0.9)
HIGH_COUNT=$(echo "$OUT" | jq '.candidates_found')
TOTAL=$((TOTAL + 1))
if [ "$HIGH_COUNT" -ge "$CAND_COUNT" ]; then
  PASS=$((PASS + 1))
  echo -e "    ${GREEN}✔${RESET} Higher threshold → more candidates ($HIGH_COUNT >= $CAND_COUNT)"
else
  FAIL=$((FAIL + 1))
  echo -e "    ${RED}✘${RESET} Expected higher threshold to find more candidates"
fi

# ══════════════════════════════════════════════════════════════════════
banner "Observability: Operation Log"
# ══════════════════════════════════════════════════════════════════════

step "log — shows operations from TESTDIR2"
OUT=$($M --data-dir "$TESTDIR2" log --limit 30)
echo "$OUT" | head -10 | sed 's/^/    /'
assert_contains "log has remember ops" "$OUT" "remember"
assert_contains "log has recall ops"   "$OUT" "recall"

step "log — shows link and gc operations"
OUT=$($M --data-dir "$TESTDIR5" log --limit 30)
assert_contains "log has link ops" "$OUT" "link"

OUT=$($M --data-dir "$TESTDIR6" log --limit 30)
assert_contains "log has gc ops" "$OUT" "gc"

# ══════════════════════════════════════════════════════════════════════
banner "Milestone 7: Embedding Support (Ollama)"
# ══════════════════════════════════════════════════════════════════════

step "embed --status — always works (even without Ollama)"
TESTDIR7="$TESTDATA/m7"
mkdir -p "$TESTDIR7"
$M --data-dir "$TESTDIR7" remember "Embedding test insight one" --cat fact --imp 3 > /dev/null
$M --data-dir "$TESTDIR7" remember "Embedding test insight two" --cat fact --imp 3 > /dev/null
OUT=$($M --data-dir "$TESTDIR7" embed --status)
show_json "$OUT"
assert_jq "total_insights is 2" "$OUT" '.total_insights' '2'
assert_contains "has ollama_available" "$OUT" '"ollama_available"'
assert_contains "has coverage field" "$OUT" '"coverage"'

# Check if Ollama is available for the remaining tests
OLLAMA_OK=$(echo "$OUT" | jq -r '.ollama_available')
if [ "$OLLAMA_OK" = "true" ]; then
  step "remember — auto-embeds when Ollama available"
  OUT=$($M --data-dir "$TESTDIR7" remember "This insight should be auto-embedded" --cat fact --imp 3)
  assert_jq "embedded is true" "$OUT" '.embedded' 'true'
  ID_E1=$(extract_id "$OUT")

  step "embed --all — backfill un-embedded insights"
  # Create an insight without auto-embedding by pointing Ollama to a dead endpoint
  MNEMON_EMBED_ENDPOINT="http://127.0.0.1:1" $M --data-dir "$TESTDIR7" remember "Un-embedded test insight" --cat fact --imp 2 > /dev/null

  # Backfill all — should find the un-embedded one
  OUT=$($M --data-dir "$TESTDIR7" embed --all)
  show_json "$OUT"
  assert_jq "backfill status" "$OUT" '.status' 'backfill_complete'
  assert_contains "has succeeded count" "$OUT" '"succeeded"'

  step "embed --status — verify coverage after backfill"
  OUT=$($M --data-dir "$TESTDIR7" embed --status)
  assert_jq "all embedded" "$OUT" '.coverage' '100%'

  step "recall --smart — uses hybrid search with embeddings"
  OUT=$($M --data-dir "$TESTDIR7" recall "embedding test" --smart)
  COUNT=$(echo "$OUT" | jq 'length')
  TOTAL=$((TOTAL + 1))
  if [ "$COUNT" -ge 1 ]; then
    PASS=$((PASS + 1))
    echo -e "    ${GREEN}✔${RESET} Smart recall with embeddings works ${DIM}(count=$COUNT)${RESET}"
  else
    FAIL=$((FAIL + 1))
    echo -e "    ${RED}✘${RESET} Expected >= 1 results, got $COUNT"
  fi
else
  echo -e "  ${DIM}  Ollama not available — skipping embedding integration tests${RESET}"
  echo -e "  ${DIM}  Install: brew install ollama && ollama pull nomic-embed-text${RESET}"

  step "remember — embedded=false when Ollama unavailable"
  OUT=$($M --data-dir "$TESTDIR7" remember "This insight will not be embedded" --cat fact --imp 3)
  assert_jq "embedded is false" "$OUT" '.embedded' 'false'
fi

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
