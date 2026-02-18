# Claude Code Memory Ecosystem Landscape

## Four Architecture Patterns

### Pattern A: Claude Code Native Plugin (Hooks + Skills)

Uses Claude Code's lifecycle hooks for capture, Claude itself as extraction engine, local storage.

| Project | Mechanism | Storage | Graph | LLM Decision |
|---------|-----------|---------|:-----:|:------------:|
| claude-mem | 5 hooks + agent-sdk extracts "learnings" | SQLite | No | Medium (auto-extract) |
| claude-supermemory | Hooks + Supermemory cloud | Cloud | No | Low |
| memory-mcp | Hooks, stores in CLAUDE.md itself | CLAUDE.md | No | Medium |

### Pattern B: MCP Server (Standalone Service)

Separate process exposes memory tools via MCP. Claude Code is a client.

| Project | Backend | Graph | LLM Decision |
|---------|---------|:-----:|:------------:|
| OpenMemory | Multi-sector cognitive model | Partial | Low |
| mcp-memory-service | ChromaDB + sentence transformers | No | Low |
| mcp-mem0 | Wraps Mem0 Python SDK | Mem0 built-in | Low (Mem0 internal LLM) |
| mem0-mcp (official) | Mem0 API | Mem0 built-in | Low |
| memcp | SQLite + MAGMA 4-graph | Full 4-graph | Zero |

### Pattern C: Pure File-Based (CLAUDE.md / Markdown)

No external services. Memory lives in files that Claude reads/writes.

| Project | Mechanism | Complexity |
|---------|-----------|:----------:|
| my-claude-code-setup | Structured markdown memory bank | Very low |

### Pattern D: LLM-Supervised CLI

Independent binary handles deterministic work, LLM supervises decisions.

| Project | Binary | Graph | LLM Decision |
|---------|--------|:-----:|:------------:|
| **Mnemon** | Go + SQLite | Full 4-graph | **High** |

## Positioning Map

```
              LLM Decision Power
              High
               │
    Mnemon ────┤
               │
               │          claude-mem
               │              │
               ├──────────────┤
               │              │
               │    claude-cognitive
               │
   memcp ──────┤──── OpenMemory ──── mcp-mem0
               │
              Low
               │
        ───────┼────────────────────────► Automation
             Manual                    Fully Auto
```

## Key Observations

1. **No project combines four-graph + LLM supervision** except Mnemon
2. **claude-mem validates hooks-based capture** — community accepts this pattern
3. **All MCP-based projects have low LLM decision power** — MCP tools tend to be opaque
4. **Graph structure is rare** — most projects use flat key-value or vector-only storage
5. **No "nano-mem0" exists** — a minimal reimplementation of Mem0's core pipeline for Claude Code

## Mnemon's Differentiation

| Dimension | Mnemon | Nearest Competitor |
|-----------|--------|-------------------|
| Four-graph architecture | Yes | memcp (but zero LLM) |
| LLM-supervised edges | Yes | None |
| Zero external dependencies | Yes (single Go binary) | my-claude-code-setup (but no graph) |
| Multi-CLI portable | Yes (CLI + adapters) | MCP projects (protocol-portable) |
| Explicit memory control | Yes (diff → remember → link) | claude-mem (auto-capture) |

## Market Gap

A true "nano-mem0" would be: ~500 lines, single-file Claude Code plugin, extract → store → retrieve pipeline, using Claude itself as the extraction engine. This doesn't exist yet and represents an opportunity — but it's a different product from Mnemon (automated vs supervised).

## Related

- [08-memcp-llm-gap-analysis.md](08-memcp-llm-gap-analysis.md) — why MCP tools ≠ LLM supervision
- [07-mcp-server-mode.md](07-mcp-server-mode.md) — Mnemon's MCP design preserves supervision
- [06-multi-cli-integration.md](06-multi-cli-integration.md) — reaching beyond Claude Code
