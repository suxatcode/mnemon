---
name: mnemon
description: >
  Persistent memory CLI for LLM agents. Provides commands to remember facts,
  recall past knowledge, check duplicates, link related memories, and manage
  memory lifecycle.
---

# mnemon — Command Reference

## Core workflow

```bash
# 1. Check for duplicates before remembering
mnemon diff "<new fact>"

# 2. Remember (based on diff suggestion)
#    ADD      → mnemon remember "<fact>" --cat <category> --imp <1-5> --entities "e1,e2"
#    CONFLICT → mnemon forget <old_id> && mnemon remember "<updated>" --cat <cat> --imp <n>
#    DUPLICATE→ skip

# 3. Link related memories (when remember outputs candidates)
mnemon link <new_id> <candidate_id> --type semantic --weight 0.85
mnemon link <source_id> <target_id> --type causal --weight 0.8 \
  --meta '{"sub_type":"causes","reason":"..."}'
```

## Commands

```bash
mnemon recall "<query>" --smart --limit 10   # intent-aware retrieval
mnemon search "<query>" --limit 10           # keyword search
mnemon diff "<new fact>"                     # duplicate/conflict check
mnemon remember "<fact>" --cat <cat> --imp <1-5> --entities "e1,e2"
mnemon forget <id>                           # soft-delete
mnemon related <id> --edge causal            # graph traversal
mnemon link <id1> <id2> --type <type> --weight <0-1>
mnemon gc --threshold 0.4                    # low-retention candidates
mnemon gc --keep <id>                        # boost retention
mnemon status                                # memory stats
mnemon log                                   # recent operations
mnemon embed --all                           # backfill embeddings (requires Ollama)
```

## Categories

| Category | What it captures | Typical importance |
|----------|-----------------|:------------------:|
| `preference` | User preferences, corrections, style choices | 4 |
| `decision` | Decisions with rationale | 5 |
| `insight` | Analysis results, root causes, comparisons | 4 |
| `fact` | Environment facts, system topology, domain context | 3 |
| `context` | Historical state, events, situational details | 3 |

## Rules

- ALWAYS `diff` before `remember` — no duplicates
- ALWAYS use `--smart` on recall
- Prefer specific categories over `general`
- Do NOT store secrets, passwords, or tokens
- Max 8,000 chars per insight — chunk longer content at semantic boundaries
