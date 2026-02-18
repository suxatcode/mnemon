# Daemon Agent Architecture: Two-tier Memory Processing

## Background

Combining insights from [01-sub-agent-memory](01-sub-agent-memory.md), [02-context-economics](02-context-economics.md), and [03-sleep-time-compute](03-sleep-time-compute.md), this document designs the concrete agent architecture for async memory extraction.

## Problem: Agent Framework Overhead

Any agent running through Claude Code's framework inherits structural overhead:

| Component | Tokens | Source |
|-----------|--------|--------|
| System prompt | ~2000-3000 | Claude Code built-in instructions |
| Tool definitions | ~2000-4000 | ~15 tools JSON schema |
| CLAUDE.md | ~500 | Project instructions |
| **Fixed tax per API call** | **~5000-7000** | Before any actual work |

Multi-turn tool calls compound this: each round-trip re-sends the full context (LLM is stateless).

**Result**: A simple "evaluate candidates and link" task costs ~18,000-25,000 tokens through Claude Code, vs ~1,200 tokens via direct API call.

This applies equally to `claude -p` (headless CLI) and Claude Code's `Task` tool — both are full Claude sessions with identical overhead structure.

## Proposed Architecture: Two-tier Agent

### Tier 1: Main Agent (Coordinator)

- **Role**: Analyze conversation transcript, decide what to remember
- **System prompt**: Minimal (~500 tokens), focused on memory extraction
- **Tools**: Full mnemon CLI (remember, link, recall, etc.)
- **Input**: Conversation segments from context queue
- **Output**: Structured JSON of insights to remember

### Tier 2: Sub-agents (Link Evaluators)

- **Role**: Evaluate candidates returned by `remember`, decide which edges to create
- **System prompt**: Minimal (~200 tokens), focused on edge evaluation
- **Tools**: Zero — output JSON only, mnemon binary executes links
- **Input**: Candidates from a single `remember` call (~400 tokens)
- **Output**: Link decisions as structured JSON
- **Execution**: Parallel, one per remember call

### Flow

```
Context Queue
     ↓
Main Agent (1 turn, ~4100 tokens)
  System: "Extract memorable insights from this transcript" (~500)
  Tools:  mnemon CLI tools (~600)
  Input:  conversation segment (~3000)
  Output: structured JSON [{content, category, importance}, ...]
     ↓
mnemon batch-remember (binary, zero tokens)
  → stores insights
  → collects all candidates
     ↓
Sub-agents × N (each 1 turn, each ~700 tokens, parallel)
  System: "Evaluate edge candidates, return link decisions" (~200)
  Tools:  none
  Input:  candidates JSON (~400)
  Output: {"links": [{"source": "id1", "target": "id2", "sub_type": "causes"}]}
     ↓
mnemon batch-link (binary, zero tokens)
  → executes all link operations
```

## Token Economics: Current Mode vs Proposed

### Assumptions

- 50-turn conversation, ~3 insights worth remembering per session
- Compounding formula: `tokens_per_turn × T × T/2` (triangular sum, T=50, T/2=25)
- Current mode: Claude performs remember/link during conversation
- Proposed mode: conversation is read-only, batch extraction post-compact

### Current Mode (LLM-supervised, in-conversation)

```
Per turn (compounding — re-sent every subsequent turn):
  Hook recall output:      ~1000 tokens  (past memory injection)
  Memory operation output:  ~600 tokens  (remember candidates, link results)
  Total per turn:          ~1600 tokens

Over 50 turns (triangular sum):
  1600 × 50 × 25 = 2,000,000 tokens

Additional costs:
  DAO reliability:          ~60% execution rate (sometimes skips remember)
  Cross-turn insight:       missed (each turn only sees current message)
```

### Proposed Mode (Two-tier daemon, post-compact)

```
Per turn (compounding — only read-path):
  Hook recall output:      ~300 tokens   (slim summary, no candidates)
  Memory operations:         0 tokens    (none during conversation)
  Total per turn:           ~300 tokens

Over 50 turns (triangular sum):
  300 × 50 × 25 = 375,000 tokens

Post-compact batch (fixed, one-time):
  Main agent:              ~4,100 tokens (1 turn, full transcript analysis)
  Sub-agents (×3):         ~2,100 tokens (each ~700, parallel)
  Total batch:             ~6,200 tokens

Grand total:               375,000 + 6,200 = 381,200 tokens
```

### Side-by-side Comparison

| Dimension | Current Mode | Proposed Mode | Delta |
|-----------|-------------|---------------|-------|
| Compounding tokens/turn | ~1600 | ~300 | **-81%** |
| 50-turn total tokens | ~2,000,000 | ~381,200 | **-81%** |
| Extraction reliability | ~60% (DAO) | 100% (deterministic) | **+67%** |
| Cross-turn insights | single turn only | full transcript | **qualitative leap** |
| Conversation interference | medium (candidates pollute context) | zero (read-only) | **eliminated** |
| Separate API cost | $0 | ~$0.003 (batch) | negligible |

### Why 81% Reduction

The savings come from **one structural change**: moving memory write operations out of the conversation loop.

```
Current:  1600 tokens/turn × compounds across 50 turns = 2M
Proposed:  300 tokens/turn × compounds across 50 turns = 375k
                                                          ────
Difference: 1300 tokens/turn saved × 50 × 25           = 1.6M tokens saved
```

The 1300 tokens/turn savings = eliminating remember candidates (~400), link outputs (~300), entity hints (~200), operation confirmations (~400) from conversation context. These move to the post-compact batch where they cost ~6,200 tokens total (fixed, non-compounding).

**The compounding effect is the multiplier**: 1300 tokens removed per turn doesn't save 1300 × 50 = 65,000 tokens — it saves 1300 × 50 × 25 = **1,625,000 tokens** because each token persists across all remaining turns.

## Batch Processing Token Breakdown

| Architecture | Total tokens | API calls | Notes |
|---|---|---|---|
| Claude Code Task/CLI | ~25,000 | 5-8 | Full agent framework overhead |
| Two-tier (naive) | ~16,000 | 2 + N | Main (2 turns) + sub-agents (2 turns each) |
| Two-tier (optimized) | ~6,000-7,000 | 1 + N | Main (1 turn, batch) + sub-agents (1 turn, JSON-only) |
| Direct API only | ~3,600 | 1 + N | No tools, no framework |

The optimized two-tier achieves ~2x of direct API cost while maintaining agent-level intelligence for the coordination layer.

## Key Design Decisions

### 1. Main Agent: 1-turn via batch command

Instead of multi-turn tool calls (analyze → remember → remember → ...), the main agent outputs a single structured JSON. A new `mnemon batch-remember` command accepts this JSON and executes all remembers in one binary invocation.

This eliminates the most expensive overhead: multi-turn context re-transmission on the main agent.

### 2. Sub-agents: Zero tools, JSON-only output

Sub-agents don't call any tools. They receive candidates as input and return link decisions as structured JSON. The mnemon binary parses the JSON and executes links.

This reduces each sub-agent to exactly 1 API call with ~700 tokens total.

### 3. Binary as orchestrator

The mnemon binary (not an LLM agent) orchestrates the pipeline:

```
binary: read queue → prepare transcript segments
binary: call Main Agent API → parse JSON response
binary: execute batch-remember → collect candidates
binary: call Sub-agent APIs (parallel) → parse JSON responses
binary: execute batch-link → done
```

LLM is used only for judgment (what to remember, which edges to create). All execution is deterministic binary code.

## Implementation Requirements

### New mnemon commands

```bash
# Accept JSON array of insights, remember all, return all candidates
mnemon batch-remember --json '[{"content":"...","category":"decision","importance":4}]'

# Accept JSON array of link decisions, create all edges
mnemon batch-link --json '[{"source":"id1","target":"id2","sub_type":"causes"}]'
```

### LLM API integration in Go binary

```go
// Direct Anthropic API call (no Claude Code framework)
func callLLM(systemPrompt, userContent string) (string, error) {
    // POST to api.anthropic.com/v1/messages
    // Model: claude-haiku (cost-efficient)
    // ~1200 tokens per call
}
```

### Daemon or hook trigger

```
Option A: Post-compact hook (simpler)
  on_compact → mnemon batch-process --transcript $PATH

Option B: Daemon with queue (when scaling needed)
  mnemon daemon start
  → watches ~/.mnemon/queue/
  → processes new transcripts
```

## Relation to Previous Ideas

| Idea | This document |
|------|--------------|
| 01: Sub-agent memory | Concrete two-tier implementation of the sub-agent concept |
| 02: Context economics | Optimized to ~7000 tokens total (vs ~25000 naive) |
| 03: Sleep-time compute | Daemon/hook trigger is the execution mechanism for sleep-time batch |

## Open Questions

1. **API key management**: Direct API calls require an Anthropic API key. Should mnemon accept `--api-key` flag or read from env (`ANTHROPIC_API_KEY`)?
2. **Local model alternative**: Could Ollama (qwen2.5/llama3) replace Haiku for sub-agent evaluation at zero API cost?
3. **Hybrid strategy**: Use Ollama for simple tasks (entity extraction, dedup) and Haiku for complex tasks (causal reasoning)?
