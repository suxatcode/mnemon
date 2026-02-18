# memcp LLM Decision Gap Analysis

## Finding

memcp implements MAGMA's **data structure** (four edge types) but not its **intelligence mechanism** (LLM-participated graph construction). The host LLM has zero decision-making power over graph structure.

## Edge-by-Edge Analysis

### Temporal — Zero LLM

```
Algorithm: 30min window, weight = max(0.1, 1 - delta_min/30), top-20
LLM role: None
MAGMA alignment: Acceptable (MAGMA also uses time-based heuristics)
```

### Entity — LLM assists extraction only

```
Extraction: regex (files/URLs/CamelCase) + optional Haiku sub-agent
Edge creation: exact entity match (case-insensitive) → auto-create, weight=1.0
LLM role: Haiku extracts entity names, but does NOT decide whether to create edges
MAGMA alignment: Partial (MAGMA uses LLM for structured entity extraction)
```

### Semantic — Zero LLM

```
Algorithm: embedding cosine >= 0.3 OR keyword overlap >= 0.1, top-3
LLM role: None (embedding is vector computation, not semantic understanding)
MAGMA alignment: Partial (MAGMA seeds with LLM, memcp uses threshold only)
```

### Causal — Zero LLM (biggest divergence)

```
Algorithm: 11 regex patterns + token overlap >= 3 with ratio >= 0.15
  Patterns: because|therefore|due to|caused by|as a result|decided to|
            chosen because|so that|in order to|leads to|results in
  Max 1 causal edge per insight, break after first match
  No sub-types (no causes/enables/prevents distinction)
  No metadata on edges
LLM role: None
MAGMA paper: 2-hop BFS → LLM inference → LEADS_TO/BECAUSE_OF/ENABLES/PREVENTS
Gap: This is the largest divergence from MAGMA
```

## Host LLM Visibility

The host LLM (Claude Opus) calling memcp tools:

| What host LLM CAN do | What host LLM CANNOT do |
|----------------------|------------------------|
| Decide whether to call `remember` | See what edges were auto-created |
| Choose query for `recall` | Approve/reject specific edges |
| Call `create_relation` manually | Know which edges are missing |
| Call `forget` | Understand graph topology |

**The host LLM is blind to graph structure.** It doesn't know what happened inside `remember`.

## Comparison: Three Implementations

| Dimension | MAGMA paper | memcp | Mnemon |
|-----------|------------|-------|--------|
| Causal inference | LLM slow-path | 11 regex patterns | Algorithm candidates → LLM link |
| Causal sub-types | LEADS_TO/BECAUSE_OF/ENABLES/PREVENTS | None | causes/enables/prevents |
| Entity extraction | LLM structured extraction | Regex + optional Haiku | Regex + dictionary + LLM --entities |
| Semantic seeding | LLM + cosine | cosine >= 0.3 only | cosine >= 0.50 auto |
| Host LLM sees graph | N/A | No | Yes (candidates output + related query) |
| Host LLM approves edges | N/A | No | Yes (diff → remember → link workflow) |

## Implication for Mnemon

memcp proves that:
1. MAGMA four-graph CAN be implemented without LLM in the pipeline
2. But the graph quality ceiling is bounded by heuristic algorithms
3. Causal edges are the weakest link — regex catches surface patterns, not semantic causality

Mnemon's LLM-Supervised approach addresses this gap:
- Binary generates candidates (recall for LLM)
- LLM reviews and decides (precision filtering)
- The "DAO participation rate" problem is real but addressable (see idea 01)

## Key Insight

memcp's architecture proves an important negative result: **exposing memory as MCP tools does not automatically make the LLM a supervisor.** If the tools are opaque (input → hidden processing → output), the LLM is just a caller, not a decision-maker. True LLM supervision requires transparent intermediate results (candidates, scores, graph state) that the LLM can reason about and act on.

## Related

- [07-mcp-server-mode.md](07-mcp-server-mode.md) — Mnemon's MCP design preserves transparency
- [01-sub-agent-memory.md](01-sub-agent-memory.md) — sub-agent for reliable edge evaluation
