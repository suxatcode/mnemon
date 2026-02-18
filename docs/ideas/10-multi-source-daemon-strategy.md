# Multi-source Daemon Strategy: Competitive Positioning

## Context

OpenClaw and NanoClaw have driven mainstream attention to personal AI assistants with persistent memory. However, analysis of their memory implementations reveals a structural gap that Mnemon can exploit.

## Competitor Memory Architecture

### OpenClaw

- **Storage**: Plain Markdown files (`memory/YYYY-MM-DD.md` + `MEMORY.md`)
- **Retrieval**: SQLite + embedding vector search + BM25 hybrid, MMR re-ranking
- **Graph**: **None** — community articles describe [memory as "broken"](https://blog.dailydoseofds.com/p/openclaws-memory-is-broken-heres)
- **Knowledge graph**: [Feature request exists](https://github.com/openclaw/openclaw/issues/2910) for Graphiti/Cognee/ZEP integration, not built-in
- **Pre-compact**: Silent agentic turn reminding Claude to write memories — DAO-mode, unreliable
- **LLM decision**: Low (auto-extract to Markdown, no structured evaluation)

### NanoClaw

- **Storage**: Per-group CLAUDE.md files (isolated filesystem)
- **Retrieval**: SQLite, minimal
- **Graph**: None
- **LLM decision**: None (Claude writes CLAUDE.md directly)
- **Core value**: WhatsApp integration + Apple container isolation, not memory quality

### memcp

- **Storage**: SQLite + MAGMA 4-graph
- **Retrieval**: 5-tier search (keyword → BM25 → fuzzy → semantic → hybrid)
- **Graph**: Full 4-graph (temporal/semantic/causal/entity)
- **LLM decision**: Zero within binary, optional Haiku sub-agent for entity extraction
- **Limitation**: MCP tool opacity means Claude can't supervise edge quality

## Positioning Map: Two Dimensions

```
              Knowledge Structure Depth
              High (Graph)
               │
    Mnemon ────┤                          ← Only: 4-graph + LLM supervision
               │
    memcp  ────┤                          ← 4-graph, zero LLM decision
               │
               ├──────────────────────────
               │
  OpenClaw ────┤  NanoClaw               ← Markdown + vector search
               │
              Low (Flat)
               │
        ───────┼────────────────────────► Data Source Breadth
             Single                    Multi-source
          (conversation)         (conversation+docs+social)
```

**All competitors are in the bottom-left quadrant.** The top-right quadrant (multi-source + graph structure) is vacant.

## Why Daemon Becomes Necessary

In the single-source scenario (Claude Code conversations only), hooks are sufficient (see [05-prompt-caching-impact](05-prompt-caching-impact.md)). But for multi-source knowledge capture:

| Requirement | Hook | Daemon |
|-------------|------|--------|
| Claude Code conversation | Yes (PreCompact) | Also yes |
| Monitor Twitter stream | **No** (no trigger source) | **Yes** (polling/WebSocket) |
| Monitor Discord channel | **No** | **Yes** |
| Watch document directory | **No** | **Yes** (fs watcher) |
| Cross-source dedup | **No** (hooks isolated) | **Yes** (global view) |
| Rate limiting across APIs | **No** | **Yes** (unified scheduler) |

Hook can only respond to Claude Code lifecycle events. When knowledge sources extend beyond Claude Code, daemon becomes a structural requirement.

## Mnemon vs OpenClaw: Path to Top-Right

OpenClaw's path: **bottom-right → top-right** (has multi-source, needs graph)
- Difficulty: High — redesign storage + retrieval from scratch
- Approach: Import Graphiti/Cognee as external graph layer
- Risk: Impedance mismatch between Markdown storage and graph retrieval

Mnemon's path: **top-left → top-right** (has graph, needs multi-source)
- Difficulty: Medium — add adapters + ingestion queue
- Approach: Daemon with source-specific adapters feeding existing graph engine
- Risk: Engineering scope creep

| Dimension | OpenClaw adds graph | Mnemon adds multi-source |
|-----------|-------------------|------------------------|
| Engineering | Redesign storage + retrieval | Add adapter + queue |
| Foundation | Markdown → graph = rewrite | Graph exists → add inputs |
| LLM supervision | Build from zero | Already validated |
| Retrieval quality | Upgrade from vector search | beam search + RRF already in place |

**Adding adapters to an existing graph engine is easier than grafting a graph onto Markdown storage.**

## Adapter Architecture

```
                    ┌─────────────────────────────────┐
                    │         mnemon daemon            │
                    │                                  │
 Claude Code ──hook──→                                 │
 Documents ──watcher──→  Ingestion Queue              │
 Twitter ──polling────→    │                           │
 Discord ──webhook────→    │                           │
 RSS ──cron───────────→    │                           │
                    │      ↓                           │
                    │   Adapter Pipeline               │
                    │    ├── Content extraction         │
                    │    ├── Chunking (8K limit)        │
                    │    ├── Cross-source dedup         │
                    │    └── Source tagging             │
                    │      ↓                           │
                    │   LLM Evaluation (batch)         │
                    │    ├── What to remember           │
                    │    └── Edge candidates → link     │
                    │      ↓                           │
                    │   mnemon remember + link          │
                    └─────────────────────────────────┘
```

### Per-adapter Characteristics

| Adapter | Trigger | Format | Frequency | LLM Need |
|---------|---------|--------|-----------|----------|
| Claude Code | PreCompact hook → transcript | Structured dialogue | Per compact (~hours) | High (content distillation) |
| Documents | fs watcher or CLI import | PDF/MD/TXT → chunks | On change | Medium (summary + entities) |
| Twitter | API polling (rate limited) | Tweets → filter noise | Minutes | Low (mostly rule-filtered) |
| Discord | WebSocket or batch pull | Messages → topic clusters | Real-time | Medium (signal from noise) |
| RSS | Cron schedule | Articles → extract key points | Hours | Medium (summarization) |

## True Differentiation

The competitive moat is NOT the daemon itself or which LLM engine it uses. It is:

1. **Graph structure quality** — four-graph with typed edges (temporal/causal/semantic/entity) vs flat document storage. Already built, OpenClaw cannot easily replicate.

2. **Cross-source association** — "The DeFi trend you followed on Twitter is causally linked to the design decision in your code conversation" — no existing tool does this.

3. **LLM-supervised edge quality** — memcp has graph but zero LLM decisions. OpenClaw has LLM but no graph. Mnemon has both.

4. **Intent-aware retrieval** — not "return similar documents" but "return causal chains + timelines + entity networks". Already implemented via beam search + intent weights.

## Implementation Roadmap

```
Phase 1: Hook optimization (now)
  ├── Slim recall hook (~300 tokens)
  ├── PreCompact → batch-remember
  └── Coverage: Claude Code conversations

Phase 2: Document ingestion CLI (near-term)
  ├── mnemon ingest --file <path>
  ├── mnemon ingest --dir <path> --watch
  ├── Chunking + source tagging
  └── Coverage: Claude Code + documents

Phase 3: Daemon + social adapters (when validated)
  ├── mnemon daemon start
  ├── Twitter, Discord, RSS adapters
  ├── Unified queue + cross-source dedup
  └── Coverage: all information sources

Phase 4: Platform integration (when ecosystem ready)
  ├── MCP server mode (for OpenClaw/other clients)
  ├── API mode (for custom integrations)
  └── Mnemon as pluggable memory backend
```

**Principle: Don't build infrastructure for data sources that don't exist yet.** Validate cross-source graph value with documents (Phase 2) before investing in real-time social adapters (Phase 3).

## Related

- [04-daemon-agent-architecture](04-daemon-agent-architecture.md) — two-tier agent design for batch processing
- [05-prompt-caching-impact](05-prompt-caching-impact.md) — quality over economics as primary motivation
- [09-memory-ecosystem-landscape](09-memory-ecosystem-landscape.md) — full ecosystem positioning
