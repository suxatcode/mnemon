# Prompt Caching Impact on Memory Architecture Economics

## Background

The token economics analysis in [02-context-economics](02-context-economics.md) and [04-daemon-agent-architecture](04-daemon-agent-architecture.md) calculated compounding costs assuming all tokens are charged at full input price. This document corrects for **Prompt Caching** — a provider-side optimization that fundamentally changes the cost picture.

## How Prompt Caching Works

In multi-turn conversations, the prefix (system prompt + tool definitions + all previous turns) is identical across API calls. Providers cache this prefix and charge repeated reads at a steep discount.

Anthropic pricing (Opus 4.6):

| Token type | Price (per MTok) | Relative to base |
|-----------|-----------------|-----------------|
| Base input | $5.00 | 1.0x |
| Cache write (5min TTL) | $6.25 | 1.25x |
| **Cache read** | **$0.50** | **0.1x** |
| Output | $25.00 | — |

**Key**: compounding tokens from previous turns are cache reads at **10% of base price**.

## Recalculated Economics (50-turn conversation, Opus 4.6)

### Current Mode (1600 tokens/turn compounding)

```
New tokens per turn:  1600 (hook recall + remember output + link results)
  → charged as cache write: 1600 × $6.25/MTok per turn

Accumulated tokens re-sent each turn:
  → charged as cache read: $0.50/MTok (not $5.00)

Cache writes:  50 × 1600 = 80k tokens × $6.25/MTok   = $0.50
Cache reads:   1600 × Σ(1..49) = 1.96M × $0.50/MTok  = $0.98
─────────────────────────────────────────────────────
Total memory compounding cost:                          $1.48
```

### Proposed Mode (300 tokens/turn + 6200 batch)

```
Cache writes:  50 × 300 = 15k tokens × $6.25/MTok     = $0.09
Cache reads:   300 × Σ(1..49) = 367k × $0.50/MTok     = $0.18
Batch (Haiku): 6200 tokens × $1.00/MTok               = $0.006
─────────────────────────────────────────────────────
Total cost:                                             $0.28
```

### Comparison: With vs Without Caching

| Scenario | Without caching | With caching | Caching savings |
|----------|----------------|-------------|----------------|
| Current mode | $10.00 | **$1.48** | -85% |
| Proposed mode | $0.96 | **$0.28** | -71% |
| Absolute gap | $9.04 | **$1.20** | Gap shrinks 7.5x |
| Relative ratio | 10.4x | **5.3x** | Advantage weakened |

**Prompt caching reduces the current mode's cost by 85%**, from $10.00 to $1.48 per 50-turn session. The absolute savings of switching to the proposed mode drops from $9.04 to $1.20.

## Where Should This Problem Be Solved?

### Provider Side — Cost (already being solved)

```
Prompt caching:     -90% on repeated prefix tokens (deployed)
Batch API:          -50% on async processing (deployed)
Model cost trends:  ~50-70% cost reduction per year (ongoing)
```

The trend is clear: token cost is a **provider-side optimization** that improves continuously without application changes. Building complex application architecture purely to save $1.20 per session is poor ROI.

### Application Side — Quality (not yet solved)

```
Problems that prompt caching CANNOT solve:

1. Extraction reliability:  60% DAO execution rate → 40% memory loss
   └─ This is a cognitive load problem, not a cost problem
   └─ Cheaper tokens don't make Claude more disciplined

2. Context pollution:       candidates/link outputs occupy attention
   └─ Cheaper tokens still pollute context quality
   └─ LLM attention is degraded regardless of token price

3. Cross-turn insights:     incremental (per-turn) vs batch (full transcript)
   └─ This is an information-theoretic gap, not an efficiency gap
   └─ A per-turn view structurally cannot see multi-turn arcs
```

### Framework: Which Side Solves What

| Dimension | Owner | Status | Notes |
|-----------|-------|--------|-------|
| Token cost | Provider | Solved (caching, batch, model pricing) | Improves automatically |
| Latency | Both | Partially solved (compact) | Fewer tokens still helps |
| **Extraction reliability** | **Application** | **Unsolved — core gap** | 60% → 100% requires architecture change |
| **Memory quality** | **Application** | **Unsolved** | Batch > incremental is qualitative |

## Corrected Motivation for Two-tier Architecture

The value of the proposed daemon/two-tier architecture should NOT be framed as a cost optimization. The correct priority ordering:

```
1. Extraction reliability    60% → 100%              ← PRIMARY motivation
   Current mode depends on Claude voluntarily executing remember/link.
   Cognitive load, context length, and task complexity all reduce compliance.
   Deterministic pipeline eliminates this variance entirely.

2. Context quality           zero pollution           ← DIRECT user benefit
   Memory operation outputs (candidates, link results, entity hints) in
   conversation context compete for LLM attention with actual dialogue.
   Read-only mode during conversation keeps context clean.

3. Cross-turn insights       full transcript review   ← QUALITATIVE improvement
   Per-turn extraction only sees the current message.
   Batch extraction sees the full conversation arc:
   "decision evolved from A to B over 3 turns" is invisible incrementally.

4. Token economics           $1.48 → $0.28            ← BONUS, not driver
   Real but modest savings. Not worth engineering effort alone.
   Will become even less significant as provider costs continue to fall.
```

## Implication for Implementation Priority

Given this analysis, the implementation roadmap should be driven by quality metrics, not cost:

**Phase 1**: Slim recall hook (reduce context pollution)
- Change hook output from full candidates to slim summary (~300 tokens)
- Immediate quality improvement, zero new infrastructure
- Does NOT require daemon or batch processing

**Phase 2**: Post-compact batch remember (ensure reliability)
- `mnemon batch-remember` triggered after compact
- Ensures 100% extraction from conversation transcript
- Simple hook trigger, no daemon needed

**Phase 3**: Daemon + sub-agent pipeline (when scaling needed)
- Only build when Phase 2 proves insufficient
- Queue management, parallel processing, failure recovery
- Justified by operational needs, not cost savings
