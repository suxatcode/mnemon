# Mnemon

A persistent memory system for LLM agents, built on MAGMA's four-graph architecture.

## What is Mnemon?

LLM agents forget everything between sessions. Context compaction drops critical decisions, cross-session knowledge vanishes, and long conversations push early information out of the window.

Mnemon gives your LLM persistent, cross-session memory — with a single Go binary and a skill file.

## Quick Start

```bash
git clone https://github.com/Grivn/mnemon.git && cd mnemon
make setup          # build + install binary + skill + hook
make claude-inject  # inject memory guidance into ./CLAUDE.md
```

That's it. Start a new Claude Code session — the hook auto-recalls relevant memories, the skill teaches command syntax, and CLAUDE.md guides when to remember.

To remove the memory guidance from CLAUDE.md: `make claude-eject`.

## How it works

Mnemon has three layers:

**The binary** — a Go CLI with SQLite storage. Handles persistence, graph indexing, keyword search, embedding, retention decay. No LLM inside, no API keys, no network calls.

**Three integration layers** teach the LLM to use the binary:

| Layer | Role | How |
|-------|------|-----|
| **[Hook](scripts/hooks/user_prompt.sh)** | Auto-recall | Runs `mnemon recall` on every user message, injects results into LLM context |
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
  Skill ── "here's how: mnemon diff → mnemon remember --cat ..."
```

### Why this design?

- **Hook handles recall reliably** — no LLM initiative required, memories appear in every conversation
- **CLAUDE.md has high authority** — project-level instructions the LLM follows more consistently than tool docs
- **Skill stays focused** — pure command reference, no behavioral logic mixed in

### Adapting for other LLM-CLIs

For non-Claude-Code tools, merge the three layers into your system prompt or rules file: copy the recall logic, behavioral guidance, and command reference into `.cursorrules`, `RULES.md`, or equivalent.

## Features

- **MAGMA four-graph architecture** — temporal, entity, causal, and semantic edges, not just vector similarity
- **LLM-supervised** — the LLM actively decides what to remember, update, link, and forget
- **Intent-aware recall** — `--smart` mode uses graph traversal + optional vector search (RRF fusion)
- **Duplicate detection** — `diff` compares new content against existing insights before storing
- **Retention lifecycle** — importance decay, access-count boosting, immunity rules, and garbage collection
- **Optional embeddings** — local Ollama integration for hybrid vector+keyword search
- **Zero external dependencies** — single binary, SQLite WAL, no API keys needed

## Usage

### Core commands

```bash
# Remember — store a new insight
mnemon remember "Chose Qdrant over Milvus for vector search" \
  --cat decision --imp 5 --entities "Qdrant,Milvus"

# Recall — retrieve insights (--smart for graph-enhanced retrieval)
mnemon recall "vector database" --smart --limit 10

# Search — token-scored keyword search
mnemon search "authentication" --limit 10

# Diff — check for duplicates/conflicts before remembering
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

### Embeddings (optional)

Requires [Ollama](https://ollama.ai) with `nomic-embed-text`:

```bash
ollama pull nomic-embed-text

mnemon embed --status    # check embedding coverage
mnemon embed --all       # backfill all insights
mnemon embed <id>        # embed a specific insight
```

When embeddings are available, `recall --smart` automatically uses hybrid vector+keyword search with RRF fusion.

## Architecture

```
┌──────────────────┐     CLI commands      ┌──────────────────┐
│   LLM Agent      │ ───────────────────── │     Mnemon       │
│ (Claude Code,    │  remember, recall,    │                  │
│  Cursor, etc.)   │  diff, link, forget   │  SQLite (WAL)    │
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

Based on [MAGMA](https://arxiv.org/abs/2601.03236) (Multi-Graph Agentic Memory Architecture). See [Design & Architecture](DESIGN.md) for details.

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
make setup          # full setup (binary + skill + hook)
make eject          # remove skill
make eject-hooks    # remove hook from Claude Code settings
make uninstall      # remove everything
make help           # show all targets
```

**Dependencies**: Go 1.24+, `modernc.org/sqlite`, `spf13/cobra`, `google/uuid`

**Optional**: [Ollama](https://ollama.ai) with `nomic-embed-text` for embedding support

## Documentation

- [Design & Architecture](DESIGN.md) — core concepts, MAGMA four-graph model, LLM-supervised architecture, algorithms, design decisions
- [Architecture Diagrams](../diagrams/) — system architecture, remember/recall pipelines, four-graph model, lifecycle management (drawio + exported images)

## License

[MIT](LICENSE)
