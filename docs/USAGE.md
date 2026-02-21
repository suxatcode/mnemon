# Mnemon — Usage & Reference

> You don't run mnemon commands yourself — the agent does, driven by hooks and guided by the skill file. This document is a reference for understanding what the agent can do, for debugging, and for advanced manual operation.

---

## CLI Commands

### Core

```bash
# Remember — store a new insight (built-in diff: duplicates skipped, conflicts auto-replaced)
mnemon remember "Chose Qdrant over Milvus for vector search" \
  --cat decision --imp 5 --entities "Qdrant,Milvus" --source agent

# Recall — intent-aware graph-enhanced retrieval (default)
mnemon recall "vector database" --limit 10

# Search — token-scored keyword search
mnemon search "authentication" --limit 10

# Forget — soft-delete an insight
mnemon forget <id>
```

### Graph Operations

```bash
# Link — create a typed edge
mnemon link <source_id> <target_id> --type semantic --weight 0.85
mnemon link <source_id> <target_id> --type causal --weight 0.8 \
  --meta '{"sub_type":"causes","reason":"..."}'

# Related — BFS traversal from an insight
mnemon related <id> --edge causal --depth 2
```

### Lifecycle Management

```bash
# GC — view low-retention candidates
mnemon gc --threshold 0.5

# GC keep — boost an insight's retention
mnemon gc --keep <id>
```

### Store Management

Mnemon supports named stores for data isolation. Each store has its own independent database.

```bash
# List all stores (* marks the active one)
mnemon store list

# Create a new store
mnemon store create work

# Switch the default active store
mnemon store set work

# Remove a store (cannot remove the active store)
mnemon store remove old-project
```

**Store resolution priority** (highest to lowest):

1. `--store <name>` CLI flag
2. `MNEMON_STORE` environment variable
3. `~/.mnemon/active` file
4. Falls back to `"default"`

Different agents or processes can use different stores via the `MNEMON_STORE` environment variable — no global state contention. Legacy databases (`~/.mnemon/mnemon.db`) are automatically migrated to `~/.mnemon/data/default/` on first run.

### Observability

```bash
mnemon status    # memory statistics
mnemon log       # operation log
```

### Visualization

Export the knowledge graph for visual exploration:

```bash
# DOT format — render with Graphviz (brew install graphviz)
mnemon viz --format dot -o graph.dot
dot -Tpng graph.dot -o graph.png

# Interactive HTML — open directly in the browser (vis.js, no install needed)
mnemon viz --format html -o graph.html
open graph.html
```

Nodes are colored by category (decision, fact, insight, preference, context); edges are colored by type (temporal, semantic, causal, entity).

---

## Embedding Support (Optional)

Mnemon works fully without Ollama — all core features (remember, recall, link, graph traversal) function out of the box. Adding Ollama enhances recall precision through vector similarity, but is never required.

### What changes with and without embeddings

| Capability | Without Ollama | With Ollama |
|---|---|---|
| **Recall anchors** | Keyword + recency | Keyword + vector + recency (RRF hybrid) |
| **Semantic edges** | Token overlap (coarser) | Cosine similarity ≥ 0.50 (precise) |
| **Traversal scoring** | Pure structural | Structural + semantic |
| **Rerank weights** | Keyword 45%, Entity 25%, Graph 30% | Keyword 30%, Entity 15%, Similarity 35%, Graph 20% |

When Ollama is unavailable, the reranking system automatically redistributes similarity weight to keyword and graph signals — no configuration needed, no degraded mode flag. The system detects Ollama availability at runtime with a 2-second timeout.

### Setup

```bash
brew install ollama              # or see https://ollama.ai
ollama pull nomic-embed-text     # download the embedding model
```

Verify with:

```bash
mnemon embed --status
```

```json
{
  "total_insights": 87,
  "embedded": 87,
  "coverage": "100%",
  "ollama_available": true,
  "model": "nomic-embed-text"
}
```

### Backfilling existing insights

If you install Ollama after already using mnemon, existing insights won't have embeddings. Backfill them in one command:

```bash
mnemon embed --all
```

This generates embeddings for all un-embedded insights and automatically creates semantic edges. You can check coverage before and after with `mnemon embed --status`.

---

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

Inspired by [MAGMA](https://arxiv.org/abs/2601.03236) four-graph model. See [Design & Architecture](DESIGN.md) for the full deep dive.
