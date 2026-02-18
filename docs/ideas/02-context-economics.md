# LLM Context Economics

## Core Fact: LLM Is Stateless

Every API call re-transmits the **entire** conversation history. Context doesn't "stay loaded" between turns.

```
Turn 1: pay for S + M₁
Turn 2: pay for S + M₁ + A₁ + M₂         (Turn 1 content re-sent)
Turn T: pay for S + Σ(all previous turns)  (everything re-sent)
```

**Any token added to conversation history incurs O(T) compounding cost** over remaining turns.

## Three Memory Architecture Cost Models

### Mem0: Context Duplication

```
Dialogue context:  +800 tokens/turn (search results injected)  → compounds
Separate API call: N + 1500 tokens/turn (N = conversation length) → doesn't compound but grows with N
```

- Dialogue context stays relatively clean
- But extraction call copies growing conversation → **O(N) per call**

### Letta: In-context Memory Block

```
Memory block: +1500-2200 tokens/turn (always in system prompt) → compounds
Tool calls:   +500-1000 tokens/turn (memory operations)        → compounds
```

- No separate API calls
- But heaviest context pollution → **~2200 tokens/turn compounding**

### Mnemon (Current): LLM-supervised

```
Hook output:      +1000 tokens/turn (past memory recall)    → compounds
Memory operations: +600 tokens/turn (remember/link outputs)  → compounds
```

- No separate API calls, no context duplication
- But hook + operation outputs compound → **~1600 tokens/turn compounding**

### Mnemon (Proposed): Sub-agent Mode

```
Hook output:       +300 tokens/turn (slim recall summary)  → compounds
Memory result:     +50 tokens/turn (one-line summary)       → compounds
Sub-agent call:    ~1000 tokens/call (fixed, isolated)      → doesn't compound
```

- Minimal context pollution → **~350 tokens/turn compounding**
- Sub-agent cost is fixed regardless of conversation length

## Cost Comparison Over 50-turn Conversation

| Architecture | Context compounding | Separate API tokens | Total extra tokens |
|---|---|---|---|
| Mem0 | 800 × 50 × 25 = 1M | (avg 10k) × 50 = 500k | **1.5M** |
| Letta | 2200 × 50 × 25 = 2.75M | 0 | **2.75M** |
| Mnemon (current) | 1600 × 50 × 25 = 2M | 0 | **2M** |
| Mnemon (sub-agent) | 350 × 50 × 25 = 437k | 1000 × 20 = 20k | **457k** |

*Compounding formula: tokens_per_turn × T × T/2 (triangular sum)*

## Mnemon's Unique Advantage: LLM-supervised Mode

Mnemon doesn't need a separate API key or billing. The main agent (Claude) already participates in memory decisions as part of the dialogue — content curation, entity hints, importance assessment are all "free" side effects of the ongoing conversation.

This is fundamentally different from Letta/Mem0 where memory LLM calls are **additional cost on top of dialogue**.

## Context Management Principle

```
Information needed every turn  → direct context (keep small: <300 tokens)
One-time processing            → sub-agent (isolated, result only)
Structured mutable state       → direct context modification (use sparingly)
Historical knowledge           → external DB + retrieval (mnemon's approach)
```

**Rule of thumb**: if the process is token-dense but the result is token-sparse, use a sub-agent.
