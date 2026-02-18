# Sleep-time Compute: Batch Memory Extraction

## Background

Reference: *Sleep-time Compute* (Letta/Berkeley, 2025.04) — separate memory maintenance from active conversation, process during "sleep time".

## Problem with Incremental Remember

Current flow: Claude tries to `remember` during conversation → unreliable (DAO problem) + context pollution.

Even with sub-agent improvement, incremental extraction has a limitation: **each turn only sees the current message, not the full conversation arc**. Cross-turn insights (evolving decisions, multi-step reasoning chains) are hard to capture incrementally.

## Proposed Architecture

### Active Time: Read-only Memory

```
Turn 1 → hook: mnemon recall → inject past memory (slim, ~300 tokens)
         Claude: pure conversation, zero memory operations
Turn 2 → hook: mnemon recall → inject past memory
         Claude: pure conversation
...
```

Main agent focuses 100% on dialogue quality. No remember, no link, no diff.

### Sleep Time: Batch Write via Sub-agents

Triggered by: approaching context limit / every N turns / session end.

```
Trigger detected →
  ├── Sub-agent A: process Turn 1-5  → extract insights → remember + link
  ├── Sub-agent B: process Turn 6-10 → extract insights → remember + link
  └── Sub-agent C: process Turn 11-N → extract insights → remember + link
  (parallel execution)
  → All return summaries
  → Context can now be safely compressed
```

### Why Batch Is Better Than Incremental

| Dimension | Incremental | Batch |
|-----------|------------|-------|
| Global view | Single turn only | Full conversation segment |
| Cross-turn insights | Missed | Captured (e.g., "decision evolved from A to B over 3 turns") |
| Causal chains | Only explicit keywords | Can trace reasoning across turns |
| Redundancy | May remember same thing multiple times | Dedup naturally across segment |
| Context pollution | Every turn | Zero during conversation |

## Trigger Mechanisms

### Option A: Periodic (every N turns)

```
if turn_count % 10 == 0:
    spawn_memory_agents(last_10_turns)
```

Simple but arbitrary. May trigger during an unfinished discussion.

### Option B: Token budget threshold

```
if estimated_context_tokens > 0.7 * context_limit:
    spawn_memory_agents(all_unprocessed_turns)
```

More precise. Ensures extraction happens before compression.

### Option C: Session end hook

```
on_session_end:
    spawn_memory_agents(full_session)
```

Most natural "sleep time". But risks losing data if session crashes.

### Recommended: B + C combined

Use token threshold as primary trigger, session end as safety net.

## Relation to MAGMA

This maps directly to MAGMA's dual-stream architecture:

| MAGMA | Mnemon Sleep-time |
|-------|-------------------|
| Fast Path: event segmentation + temporal edges | Hook recall (read-only, during conversation) |
| Slow Path: async LLM consolidation | Batch sub-agent extraction (before compact) |

The key insight: MAGMA's Fast/Slow separation is not just about sync/async — it's about **separating read-path from write-path** to keep the active loop responsive.

## Implementation Considerations

1. **Sub-agent input**: conversation transcript segments (can be extracted from Claude Code's context)
2. **Sub-agent prompt**: structured extraction template (what to remember, how to categorize, which candidates to evaluate)
3. **Parallelism**: independent segments can be processed concurrently
4. **Idempotency**: `mnemon diff` before `remember` to avoid duplicates across segments
5. **Model choice**: Haiku for cost efficiency, Sonnet if causal reasoning quality is critical
