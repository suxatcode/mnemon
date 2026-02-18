# Mnemon

A persistent memory system for LLM agents, built on MAGMA's four-graph architecture.

## What is Mnemon?

LLM agents forget everything between sessions. Context compaction drops critical decisions, cross-session knowledge vanishes, and long conversations push early information out of the window.

Mnemon gives your LLM persistent, cross-session memory — with a single Go binary and a skill file.

## Quick Start

```bash
git clone https://github.com/Grivn/mnemon.git && cd mnemon
make setup    # build + install binary + install skill
```

That's it. Start a new Claude Code session — Claude auto-discovers the skill and begins recalling context and remembering facts across sessions.

## How it works: Binary + Skill

Mnemon has two parts:

**The binary** — a Go CLI with SQLite storage. Handles persistence, graph indexing, keyword search, embedding, retention decay. No LLM inside, no API keys, no network calls.

**The [skill](skills/mnemon/SKILL.md)** — a markdown file that teaches the LLM *when* and *how* to call the binary. It's a natural language memory protocol:

```markdown
# What the skill tells the LLM:

On conversation start → mnemon recall "<topic>" --smart
When a fact is learned → mnemon diff → mnemon remember
When insights are related → mnemon link --type causal
When memory gets stale → mnemon gc
```

The skill is not code. It's ~100 lines of instructions that any LLM can follow. `make setup` copies it to `~/.claude/skills/mnemon/` where Claude Code auto-discovers it.

### Why a skill, not an SDK or MCP server?

The LLM is already the smartest component in the system. It doesn't need an SDK to call `mnemon remember` — it needs to know *when* to call it and *what* to pass. That's what the skill provides.

| Approach | What it requires | LLM involvement |
|----------|-----------------|-----------------|
| **SDK/library** | Language bindings, import, API wrapper | LLM calls wrapper functions |
| **MCP server** | Protocol implementation, long-running process | LLM calls MCP tools |
| **Skill + CLI** | A markdown file + a binary in PATH | LLM reads instructions, runs shell commands |

The skill approach means:
- **Zero protocol overhead** — no MCP handshake, no tool registration, no JSON schema
- **Portable** — any LLM-CLI that can run shell commands works (Claude Code, Cursor, Windsurf, etc.)
- **LLM is the intelligent layer** — entity extraction, causal reasoning, deduplication judgment all happen in the LLM, not in embedded small models
- **Inspectable** — the entire integration is a readable markdown file, not compiled code

### Adapting for other LLM-CLIs

The skill at [`skills/mnemon/SKILL.md`](skills/mnemon/SKILL.md) is plain markdown. For non-Claude-Code tools, copy its content into your `.cursorrules`, system prompt, or rules file.

## Features

- **MAGMA four-graph architecture** — temporal, entity, causal, and semantic edges, not just vector similarity
- **CLI-in-the-loop** — the LLM actively decides what to remember, update, link, and forget
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

Based on [MAGMA](https://arxiv.org/abs/2601.03236) (Multi-Graph Agentic Memory Architecture). See [Design & Philosophy](docs/design.md) for details.

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
make setup          # full setup (binary + skill + hooks)
make eject          # remove skill + hooks
make uninstall      # remove everything
make help           # show all targets
```

**Dependencies**: Go 1.24+, `modernc.org/sqlite`, `spf13/cobra`, `google/uuid`

**Optional**: [Ollama](https://ollama.ai) with `nomic-embed-text` for embedding support

## Documentation

- [Design & Philosophy](docs/design.md) — naming origin, MAGMA four-graph design, CLI-in-the-loop architecture
- [Architecture Analysis](docs/analysis/) — detailed comparison with MAGMA paper, sequence diagrams, tradeoff assessment

## License

[MIT](LICENSE)
