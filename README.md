<p align="center">
  <img src="docs/logo/logo.svg" width="160" height="160" alt="Mnemon Logo" />
</p>

# Mnemon

**Persistent memory for LLM agents** — LLM-supervised, skill-integrated, four-graph architecture.

[![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![CI](https://github.com/mnemon-dev/mnemon/actions/workflows/ci.yml/badge.svg)](https://github.com/mnemon-dev/mnemon/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

---

LLM agents forget everything between sessions. Context compaction drops critical decisions, cross-session knowledge vanishes, and long conversations push early information out of the window.

Mnemon gives your LLM persistent, cross-session memory — with a single Go binary and a skill file.

### Why Mnemon?

Memory has a **compound interest effect** — the longer it accumulates, the more valuable it becomes. LLM engines keep iterating, skills cost nothing to write, but memory is a private asset that grows with the user. It is the only component in the agent ecosystem worth deep investment.

Mnemon is built on one principle: **the LLM itself is the best orchestrator.** Instead of embedding a small LLM inside the pipeline, Mnemon lets your host LLM — the one already in your conversation with full context — serve as the supervisor. The binary is the organ (deterministic storage, graph indexing, search, decay); the LLM is the brain (decides what to remember, how to link, when to forget). A skill file is the textbook that teaches the protocol.

This means: **memory management logic moves from prompt to code — deterministic, testable, portable.** The same binary + skill works across Claude Code, Cursor, or any LLM CLI that reads markdown.

| Pattern | LLM Role | Representative |
|---|---|---|
| **LLM-Embedded** | Executor inside the pipeline | Mem0, MAGMA |
| **MCP Server** | Tool provider via MCP protocol | MemCP |
| **LLM-Supervised** | External supervisor over a standalone binary | Mnemon |

See [Design & Architecture](docs/DESIGN.md) for the full philosophy.

## Quick Start

```bash
go install github.com/mnemon-dev/mnemon@latest
mnemon setup        # detect environments, deploy skill + hooks
```

Or from source:

```bash
git clone https://github.com/mnemon-dev/mnemon.git && cd mnemon
make install && mnemon setup
```

That's it. `mnemon setup` auto-detects Claude Code (and OpenClaw), deploys hooks and skills, and optionally injects memory guidance into your project's CLAUDE.md.

Start a new LLM CLI session — the hook auto-recalls relevant memories, the skill teaches command syntax, and CLAUDE.md guides when to remember.

To remove all integrations: `mnemon setup --eject`.

## How it works

Mnemon has three layers:

**The binary** — a Go CLI with SQLite storage. Handles persistence, graph indexing, keyword search, embedding, retention decay. No LLM inside, no API keys, no network calls.

**Three integration layers** teach the LLM to use the binary:

| Layer | Role | How |
|-------|------|-----|
| **[Hook (recall)](scripts/hooks/user_prompt.sh)** | Auto-recall | Runs `mnemon recall` on every user message, injects results into LLM context |
| **[Hook (stop)](scripts/hooks/stop.sh)** | Memory reminder | After each response, reminds the LLM to consider remembering |
| **[CLAUDE.md](CLAUDE.md)** | Behavioral guidance | Tells the LLM *when* to use recalled memories and *when* to remember new ones |
| **[Skill](skills/mnemon/SKILL.md)** | Command reference | Documents command syntax, categories, workflow |

```
User message
    │
    ▼
  Hook ─── auto-recall ──→ [Past memory] injected into context
    │
    ▼
  CLAUDE.md ── "use past memory; evaluate remember after responding"
    │
    ▼
  Skill ── "here's how: mnemon remember --cat ... (diff built-in)"
    │
    ▼
  Sub-agent ── main LLM delegates; sub-agent reads Skill, executes commands
```

### Why this design?

- **Hook handles recall reliably** — no LLM initiative required, memories appear in every conversation
- **CLAUDE.md has high authority** — project-level instructions the LLM follows more consistently than tool docs
- **Skill stays focused** — pure command reference, no behavioral logic mixed in
- **Sub-agent isolates cost** — memory writes run in a lightweight sub-agent (~1000 tokens), not the main conversation (~25000 tokens)

### Adapting for other LLM-CLIs

For non-Claude-Code tools, merge the three layers into your system prompt or rules file: copy the recall logic, behavioral guidance, and command reference into `.cursorrules`, `RULES.md`, or equivalent.

## Features

- **LLM-supervised** — the host LLM actively decides what to remember, update, link, and forget; no embedded LLM, no extra API calls
- **Skill-integrated** — a single skill file teaches any LLM CLI the full command protocol; works with Claude Code, Cursor, or anything that reads markdown
- **Four-graph architecture** — temporal, entity, causal, and semantic edges, not just vector similarity
- **Intent-aware recall** — graph traversal + optional vector search (RRF fusion), default for all queries
- **Built-in deduplication** — `remember` automatically detects duplicates and conflicts; skips or auto-replaces
- **Retention lifecycle** — importance decay, access-count boosting, immunity rules, and garbage collection
- **Optional embeddings** — local Ollama integration for hybrid vector+keyword search
- **Graph visualization** — export as Graphviz DOT or interactive vis.js HTML

## Usage

### Core commands

```bash
# Remember — store a new insight (built-in diff: duplicates skipped, conflicts auto-replaced)
mnemon remember "Chose Qdrant over Milvus for vector search" \
  --cat decision --imp 5 --entities "Qdrant,Milvus" --source agent

# Recall — intent-aware graph-enhanced retrieval (default)
mnemon recall "vector database" --limit 10

# Search — token-scored keyword search
mnemon search "authentication" --limit 10

# Diff — standalone duplicate/conflict check (optional; remember has this built-in)
mnemon diff "New fact to check"

# Forget — soft-delete an insight
mnemon forget <id>
```

### Graph operations

```bash
# Link — create typed edges between insights
mnemon link <source_id> <target_id> --type semantic --weight 0.85
mnemon link <source_id> <target_id> --type causal --weight 0.8 \
  --meta '{"sub_type":"causes","reason":"..."}'

# Related — BFS traversal from an insight
mnemon related <id> --edge causal --depth 2
```

### Lifecycle management

```bash
# GC — review low-retention insights
mnemon gc --threshold 0.5

# GC keep — boost retention for an insight
mnemon gc --keep <id>
```

### Observability

```bash
mnemon status    # memory statistics
mnemon log       # recent operations
```

### Visualization

Export the knowledge graph for visual exploration:

```bash
# DOT format — render with Graphviz (brew install graphviz)
mnemon viz --format dot -o graph.dot
dot -Tpng graph.dot -o graph.png

# Interactive HTML — open directly in browser (vis.js, no install needed)
mnemon viz --format html -o graph.html
open graph.html
```

Nodes are color-coded by category (decision, fact, insight, preference, context). Edges are color-coded by type (temporal, semantic, causal, entity).

### Embeddings (optional)

Requires [Ollama](https://ollama.ai) with `nomic-embed-text`:

```bash
ollama pull nomic-embed-text

mnemon embed --status    # check embedding coverage
mnemon embed --all       # backfill all insights
mnemon embed <id>        # embed a specific insight
```

When embeddings are available, `recall` automatically uses hybrid vector+keyword search with RRF fusion.

## Architecture

```
┌──────────────────┐     CLI commands      ┌──────────────────┐
│   LLM Agent      │ ───────────────────── │     Mnemon       │
│ (Claude Code,    │  remember, recall,    │                  │
│  Cursor, etc.)   │  link, forget, gc     │  SQLite (WAL)    │
└──────────────────┘                       │  ┌────────────┐  │
                                           │  │ Insights   │  │
        The LLM decides WHAT               │  ├────────────┤  │
        to remember and link.              │  │ 4 Edge     │  │
                                           │  │ Types:     │  │
        Mnemon handles HOW                 │  │ temporal   │  │
        to store, index, and               │  │ entity     │  │
        retrieve.                          │  │ causal     │  │
                                           │  │ semantic   │  │
      ┌──────────────────┐                 │  ├────────────┤  │
      │  Ollama          │  (optional)     │  │ Embeddings │  │
      │  nomic-embed-text│ ◄───────────── │  └────────────┘  │
      └──────────────────┘                 └──────────────────┘
```

Inspired by the [MAGMA](https://arxiv.org/abs/2601.03236) four-graph model. See [Design & Architecture](docs/DESIGN.md) for details.

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `MNEMON_DATA_DIR` | `~/.mnemon` | Database directory |
| `MNEMON_EMBED_ENDPOINT` | `http://localhost:11434` | Ollama API endpoint |
| `MNEMON_EMBED_MODEL` | `nomic-embed-text` | Embedding model name |

Or use the `--data-dir` flag on any command.

## Development

```bash
make build          # build binary
make install        # build + install to $GOBIN
make test           # run E2E test suite
mnemon setup        # interactive setup (replaces 'make setup')
mnemon setup --eject  # remove all integrations
make sync-assets    # sync source files into embedded assets
make help           # show all targets
```

**Dependencies**: Go 1.24+, `modernc.org/sqlite`, `spf13/cobra`, `google/uuid`

**Optional**: [Ollama](https://ollama.ai) with `nomic-embed-text` for embedding support

## Documentation

- [Design & Architecture](docs/DESIGN.md) — core concepts, MAGMA four-graph model, LLM-supervised architecture, algorithms, design decisions
- [Architecture Diagrams](docs/diagrams/) — system architecture, remember/recall pipelines, four-graph model, lifecycle management (drawio + exported images)
- [中文文档](docs/zh/) — Chinese version of README and DESIGN

## License

[MIT](LICENSE)
