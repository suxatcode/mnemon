# Sub-agent Memory Processing

## Problem

Current LLM-supervised mode relies on the main agent (Claude) to evaluate causal/semantic candidates and execute `remember`/`link` commands. This suffers from the "DAO participation rate" problem — the main agent sometimes skips memory operations due to cognitive load.

Additionally, memory operation outputs (candidates JSON, link results) pollute the main conversation context, causing O(T) compounding cost due to LLM's stateless nature.

## Proposal

Spawn a lightweight sub-agent to handle memory processing when `remember` is triggered.

```
Main Agent (Opus) ─── mnemon remember "..." ──→ mnemon binary
                                                    ↓
                                               Sub-agent (Haiku)
                                               Input: {
                                                 new_insight,
                                                 causal_candidates,
                                                 semantic_candidates,
                                                 entity_hints
                                               }
                                               ~1000 tokens (fixed)
                                                    ↓
                                               Evaluate + decide
                                                    ↓
                                               mnemon auto-creates edges
                  ←── result summary ────────────
                      "stored, 3 causal + 2 entity edges"
                      ~50 tokens
```

## Why This Works

| Dimension | Current Mode | Sub-agent Mode |
|-----------|-------------|----------------|
| Reliability | ~60% (DAO problem) | 100% (deterministic) |
| Main context pollution | ~1600 tokens/turn | ~50 tokens/turn |
| Edge quality | Rule-based only | LLM-evaluated |
| Cost per call | $0 (but compounding) | ~$0.0005 (Haiku, fixed 1000 tokens) |
| Monthly cost | Hidden in context bloat | ~$0.75 |

## Key Insight

The sub-agent context is **fixed at ~1000 tokens** regardless of conversation length. This avoids both:
- Mem0's O(N) context duplication problem (copies growing conversation to separate API call)
- Current Mnemon's compounding problem (memory outputs accumulate in conversation history)

## Relation to MAGMA

This effectively implements MAGMA's **Slow Path** (`ℰ_new = Φ_Reason(N(nₜ), H_history)`):
- MAGMA: async worker → 2-hop neighborhood → LLM inference → add edges
- Mnemon sub-agent: remember trigger → candidates → Haiku evaluation → auto-create edges

Same architecture, different execution model.

## Implementation Path

1. `remember` command generates candidates (already implemented)
2. Add LLM evaluation step: call Haiku/Ollama with candidates + evaluation prompt
3. Parse structured JSON response: which causal links to create, entities to extract
4. Auto-create edges based on sub-agent decisions
5. Return summary to main agent
