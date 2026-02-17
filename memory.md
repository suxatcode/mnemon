# Memory Skill — mnemon

You have access to a persistent memory system via the `mnemon` CLI.
You MUST actively use it to store and retrieve knowledge across sessions.

## On every conversation start (MANDATORY)
```bash
mnemon recall "<topic or project name>" --smart --limit 5
```
Load relevant context before responding.

## When to remember (MANDATORY — do not skip)

You MUST run `mnemon diff` + `mnemon remember` when ANY of these occur:

1. **User states a preference** — tool choice, coding style, workflow, naming convention
2. **An architectural or design decision is made** — why X over Y, trade-offs discussed
3. **A bug is diagnosed and fixed** — root cause, fix approach, lessons learned
4. **A new project pattern is established** — file structure, API convention, testing approach
5. **User corrects you** — the correction itself is a preference or fact worth saving
6. **Key facts are discovered** — API specs, version constraints, environment details, research findings, market data, or any facts related to topics the user is tracking
7. **A task is completed** — summarize what was built/changed for future context
8. **User expresses ongoing interest in a topic** — save as preference; then treat ALL significant findings about that topic as key facts worth saving

### Judgment rules for saving
- When in doubt, **save**. The cost of a redundant `diff` is near zero; the cost of lost context is a full re-search next session.
- "Publicly searchable" is NOT a reason to skip saving. If the user cared enough to ask, the result has context value.
- For topics the user is actively tracking, save key developments, metrics, and status changes — these form a timeline that adds up across sessions.

### How to remember
```bash
# 1. ALWAYS check for duplicates first
mnemon diff "<new fact>"
# 2. Based on suggestion:
#    ADD      → mnemon remember "<fact>" --cat <category> --imp <1-5>
#    CONFLICT → mnemon forget <old_id> && mnemon remember "<updated>" --cat <cat> --imp <n>
#    DUPLICATE→ skip
```

## When the user asks about past context
```bash
mnemon recall "<query>" --smart --limit 10
```

## Categories
- `preference` — user likes/dislikes, tool choices, workflow preferences
- `decision` — architectural or design decisions with rationale
- `fact` — objective information, benchmarks, specs, environment details
- `insight` — patterns, lessons learned, debugging techniques
- `context` — project state, current phase, WIP status
- `general` — anything else

## Importance scale
- `5` critical — core architectural decisions, strong user preferences
- `4` high — important facts, recurring patterns
- `3` medium — general context (default)
- `2` low — minor details
- `1` trivial — ephemeral notes

## Semantic linking (after remember)

When `mnemon remember` outputs `semantic_candidates`, evaluate each candidate:
```bash
# For each semantically related candidate:
mnemon link <new_id> <candidate_id> --type semantic --weight 0.85
# weight guide: 0.3 (weak) → 0.6 (moderate) → 0.95 (strong)
# Skip candidates with only lexical overlap but no real semantic relation
```

## Causal linking (MANDATORY after remember when candidates exist)

When `mnemon remember` outputs non-empty `causal_candidates`, you MUST evaluate them.
Candidates include both keyword-matched (explicit) and embedding-matched (implicit) pairs.

**For each candidate, answer three questions:**
1. **Is there a real causal relationship?** (not just topic overlap)
2. **What is the direction?** Source causes/enables/prevents Target
3. **What sub_type?** causes (A led to B), enables (A made B possible), prevents (A stopped B)

```bash
# For confirmed causal relationships:
mnemon link <source_id> <target_id> --type causal --weight 0.8 \
  --meta '{"sub_type":"causes","reason":"..."}'
# weight guide: 0.6 (weak/indirect) → 0.8 (clear) → 0.95 (direct/strong)
# Skip candidates where overlap is coincidental (no real causation)
```

**Implicit candidates** (marked `causal_signal: "(implicit: embedding similarity)"`) are
especially important — they catch decision→outcome pairs that lack explicit causal words.
Evaluate these carefully: they are often real causal links that heuristics cannot auto-detect.

## Entity enrichment (after remember)

When `mnemon remember` shows `entities` in output, review if important entities were missed:
```bash
# For domain concepts, project names, people, technologies not captured by regex:
mnemon enrich <id> --entities "Entity1,Entity2" --rebuild-edges
# Skip if regex already captured all meaningful entities
```

## Narrative consolidation (periodic)

Trigger when: 20+ insights without review, or user asks to organize memories.
```bash
# 1. Find narrative clusters
mnemon consolidate --window 72h --min-cluster 3
# 2. Review each cluster — does it represent a coherent narrative?
# 3. For valid narratives:
mnemon consolidate --create --title "..." --members "id1,id2,id3"
# 4. Skip clusters that are just temporal coincidence
```

## Retention review (periodic)

Trigger when: 50+ insights, or 10+ conversations since last review, or user mentions memory clutter.
```bash
# 1. Get low-retention candidates
mnemon gc --threshold 0.4

# 2. For each candidate, decide:
#    - importance >= 4 decisions/preferences → usually keep
#    - stale context notes → usually purge
#    - outdated facts → purge or update
mnemon forget <id>          # purge
mnemon gc --keep <id>       # boost retention (+3 access, refresh timestamp)
```

## Embedding (optional enhancement)

When Ollama is running locally, `remember` auto-embeds insights and `recall --smart` uses hybrid vector+keyword search (RRF fusion). No action needed — it's automatic.
```bash
mnemon embed --status          # check coverage
mnemon embed --all             # backfill existing insights
```

## Other commands
```bash
mnemon search "<query>" --limit 10    # token-scored search
mnemon related <id> --edge causal     # find causally related insights
mnemon link <src> <tgt> --type semantic --weight 0.8  # create edge
mnemon enrich <id> --entities "X,Y" --rebuild-edges   # supplement entities
mnemon consolidate [--window 72h] [--min-cluster 3]   # find narrative clusters
mnemon consolidate --create --title "..." --members "id1,id2,id3"  # create narrative
mnemon gc [--threshold 0.4]           # review retention candidates
mnemon gc --keep <id>                 # boost retention for an insight
mnemon embed --status                 # embedding coverage
mnemon embed --all                    # backfill embeddings
mnemon status                         # memory statistics
mnemon log                            # recent operations
```

## Memory + WebSearch coordination

### When to skip web search
If `mnemon recall` returns sufficient, non-time-sensitive results — **use them directly**.
Do NOT re-search what you already know. Memory exists to avoid redundant searches.

### When to search despite having memory
- **Time-sensitive data** (prices, versions, news, status) — always verify with a fresh search, even if recall has results. Use the recalled data as a comparison baseline.
- **User explicitly asks to search** — respect the request regardless of memory state.
- **Memory is vague or incomplete** — search to fill gaps, then remember the new findings.

### Use memory to improve search quality
When you do search, use recalled context to form **more specific queries**.
Example: recall tells you the user is tracking Qdrant → search "Qdrant v2.0 release 2026" instead of "vector database news".

### Mandatory workflow order
When a response involves WebSearch, WebFetch, or any external research:
1. `mnemon recall` — load existing context, decide if search is needed
2. Perform searches / fetches (use recalled context to refine queries)
3. **`mnemon diff` + `mnemon remember` for ALL new facts — BEFORE composing the reply**
4. Respond to the user

NEVER skip step 3. NEVER defer saving to "after the reply". The reply itself is NOT the end of the workflow — persisting knowledge is.

## Rules
- ALWAYS `diff` before `remember` to avoid duplicates
- Use `--smart` on recall for intent-aware retrieval
- Prefer specific categories over `general`
- Set importance >= 4 for decisions and strong preferences
- Do NOT store secrets, passwords, or tokens
