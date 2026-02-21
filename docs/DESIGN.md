# Mnemon — Design & Architecture

> **Mnemon** (/ˈniːmɒn/), from Ancient Greek μνήμων (mnemon), formed by μνάομαι ("to remember") and the agent suffix -μων, meaning "one who remembers, a person of good memory." Homer uses "καὶ γὰρ μνήμων εἰμί" ("I remember it well") in the *Odyssey* to describe this quality. In the city-states of Ancient Greece, Mnemones were officials dedicated to record-keeping, serving as witnesses and archivists in property transactions and legal proceedings — institutional memory carriers during the transition from oral tradition to written records.
>
> The word shares its root with Mnemosyne (Μνημοσύνη), the goddess of memory — from her union with Zeus the nine Muses were born, symbolizing memory as the wellspring of all knowledge and creativity.

Mnemon is a persistent memory system designed for LLM agents. It implements the four-graph architecture from the [MAGMA](https://arxiv.org/abs/2601.03236) (Multi-Graph Agentic Memory Architecture) paper as a single Go binary + SQLite, with no external API dependencies.

This document describes Mnemon's design philosophy, core concepts, system architecture, and key algorithms in detail.

---

## Table of Contents

- [1. Vision & Problem](#1-vision--problem)
- [2. Design Philosophy](#2-design-philosophy)
- [3. Core Concepts](#3-core-concepts)
- [4. System Architecture](#4-system-architecture)
  - [4.1 Data Directory Layout](#41-data-directory-layout)
  - [4.2 Store Isolation](#42-store-isolation)
- [5. MAGMA Four-Graph Model](#5-magma-four-graph-model)
- [6. Write Pipeline: Remember](#6-write-pipeline-remember)
- [7. Read Pipeline: Smart Recall](#7-read-pipeline-smart-recall)
- [8. Deduplication & Conflict Detection: Diff](#8-deduplication--conflict-detection-diff)
- [9. Lifecycle Management](#9-lifecycle-management)
- [10. Embedding Support](#10-embedding-support)
- [11. LLM CLI Integration](#11-llm-cli-integration)
- [12. Design Decisions & Trade-offs](#12-design-decisions--trade-offs)

---

## 1. Vision & Problem

### 1.1 Memory Is the Soul of an Agent

Without reliable long-term memory, an LLM agent can never evolve from a mere "tool" into a true "assistant."

The memory layer has a **compound interest effect** — the longer it is used, the more it accumulates, and the greater its value. It is the only component in the agent ecosystem that requires deep investment and cannot be replaced: LLM engines will continue to iterate (Anthropic/OpenAI/Google, etc.), Skills have near-zero marginal cost (just write markdown), but memory is a private asset that accumulates alongside the user over time.

### 1.2 The "Amnesia" Problem of LLMs

LLM agents suffer from three critical memory deficiencies:

- **Context compression loss**: After `/compact` or automatic compression, all prior decisions, discoveries, and context are lost
- **Cross-session forgetting**: Each new session starts from scratch, with no knowledge of previous sessions
- **Long-session decay**: Once the context window fills up, critical early information is pushed out of the attention range

For a digital assistant that needs to "continuously learn the user's thinking and become an extension of the user," these three deficiencies mean users must repeatedly restate preferences, re-explain project context, and re-derive conclusions already reached.

### 1.3 Structural Bottlenecks of Traditional Approaches

Existing RAG/Memory solutions have fundamental design limitations:

1. **Memory is an afterthought** — its lifecycle is tied to the agent session, not an independent entity
2. **Writing is reactive** — summaries are extracted after conversation ends, losing structural information
3. **Retrieval is flat** — relying solely on vector similarity, unable to express temporal/causal/contradictory relationships
4. **No forgetting mechanism** — either remember everything or TTL-based blanket expiration, no intelligent decay
5. **Heavy dependencies** — requires API keys, external databases, network connections

### 1.4 Mnemon's Mission

Mnemon's goal is: **to make an LLM remember your decisions, understand your preferences, and track project context like an experienced assistant — across arbitrarily many sessions.**

It is not a library or plugin embedded within an agent framework, but a standalone memory engine — callable via the command line by Claude Code, Cursor, or any LLM CLI.

### 1.5 Comparison with Alternatives

| Dimension | Mem0 | Letta/MemGPT | MemCP | **Mnemon** |
|-----------|------|-------------|-------|-----------|
| **Architecture** | SDK embedded in call chain | Within agent framework | MCP Plugin | Standalone Binary |
| **LLM Role** | Internal extraction function | Agent self-managed | Sub-agent orchestration | External supervisor |
| **Graph** | Neo4j single relation edges | None | MAGMA four-graph | MAGMA four-graph |
| **External Deps** | PostgreSQL + LLM API | PostgreSQL + LLM API | None | None |
| **LLM Swappable** | Tied to OpenAI | Tied to framework | Tied to Claude Code | Any LLM CLI |
| **Memory Lifecycle** | Rules engine | No built-in decay | 3-zone (Active/Archive/Purge) | EI decay + GC + immunity |

---

## 2. Design Philosophy

### 2.1 LLM-Supervised: Binary as Organ, LLM as Supervisor

Traditional LLM memory systems (such as Mem0 and the original MAGMA implementation) embed a small LLM inside the pipeline to handle memory operations — entity extraction, conflict detection, causal reasoning. This is the **LLM-Embedded** pattern.

Mnemon adopts the **LLM-Supervised** pattern:

| Pattern | Where is the LLM | What does the LLM do | Representative |
|---------|------------------|---------------------|----------------|
| **LLM-Embedded** | Inside the pipeline | Executor (extraction, classification, reasoning) | Mem0, MAGMA |
| **MCP Server** | Tool provider via MCP protocol | Exposes memory operations as MCP tools for the host LLM | MemCP |
| **LLM-Supervised** | Outside the pipeline | Supervisor (reviews candidates, makes judgments, decides trade-offs) | Mnemon |

Under the LLM-Supervised pattern, responsibilities are clearly separated into two tiers:

| Tier | Role | Handles |
|------|------|---------|
| **Binary (organ)** | Deterministic computation | Storage, graph indexing, keyword search, vector math, decay formulas, auto-pruning |
| **Host LLM (supervisor)** | High-value judgment | Causal chain evaluation, semantic relevance judgment, entity enrichment, memory retention decisions |

This means:

- **Zero additional API cost**: All computation happens locally
- **Stronger judgment capability**: An Opus-class LLM evaluates candidate links, not gpt-4o-mini
- **LLM swappable**: The same Binary + Skill works across Claude Code, Cursor, or any LLM CLI

### 2.2 Tools are Organs, Skills are Textbooks

This philosophy can be understood through a game development analogy:

| Game Development | Agent Ecosystem | Mnemon Equivalent |
|-----------------|-----------------|-------------------|
| Game engine (Unity/Unreal) | LLM CLI (Claude Code/Cursor) | Host environment |
| Native plugin (C++ Plugin) | Binary tool | `mnemon` binary |
| Script/Blueprint (C#/Blueprint) | Skill (.md definition) | `SKILL.md` command reference |
| Gameplay logic | Agent behavior config | `guide.md` execution manual |

- **Binary = Organ** — defines what *can* be done. Encapsulates storage, graph traversal, lifecycle management, and other deterministic capabilities
- **Skill (.md) = Textbook** — defines *how* to do it. Teaches the LLM when to retrieve memories, how to judge deduplication, and which commands to invoke

Binary encapsulates all logic that does not require an LLM; Skill only teaches the LLM the parts that require intelligent judgment. **Memory management logic moves from prompt to code — deterministic, testable, portable.**

### 2.3 Key Insights

- **No need to build the engine layer yourself** — major vendors continuously optimize LLMs and CLI tools; developers just adopt and use them
- **Skills have near-zero marginal cost** — defining agent behavior via markdown is like game blueprints enabling non-programmers to participate
- **The memory layer is the only part worth deep investment** — memory has a compound interest effect; it is the dividing line between an agent as a "tool" versus an "assistant"
- **The LLM itself is the best orchestrator** — no need for Python DAG orchestration of call chains; the LLM reads the Skill and knows what to do

![LLM-Supervised Architecture](diagrams/05-llm-supervised.jpg)

![System Architecture](diagrams/01-system-architecture.jpg)

---

## 3. Core Concepts

![Insight & Edge Data Model](diagrams/09-insight-edge-datamodel.jpg)

### 3.1 Insight (Memory Node)

An Insight is the fundamental memory unit in Mnemon. Each insight represents an independent piece of knowledge:

```
┌─────────────────────────────────────────────┐
│ Insight                                     │
├─────────────────────────────────────────────┤
│ id         : UUID                           │
│ content    : "Chose Qdrant over Milvus..."  │
│ category   : decision                       │
│ importance : 5  (1-5)                       │
│ tags       : ["vector-db", "architecture"]  │
│ entities   : ["Qdrant", "Milvus"]           │
│ source     : "user"                         │
│ access_count        : 3                     │
│ effective_importance : 0.85                  │
│ created_at : 2026-02-18T10:00:00Z           │
└─────────────────────────────────────────────┘
```

**Categories** are divided into six types that help distinguish the nature of a memory:

| Category | Meaning | Example |
|----------|---------|---------|
| `preference` | User preference | "Prefers communicating in Chinese" |
| `decision` | Architectural/technical decision | "Chose SQLite over PostgreSQL" |
| `fact` | Objective fact | "API rate limit is 100 req/s" |
| `insight` | Reasoning conclusion | "Beam search is more suitable than full BFS for..." |
| `context` | Project context | "Phase 3 completed, 118 tests passing" |
| `general` | General | Content that doesn't fit the above categories |

**Importance** ranges from 1 to 5 and affects retrieval ranking and lifecycle:

- **5**: Critical decision, never automatically cleaned up
- **4**: Important fact, immune to auto-pruning
- **3**: Standard memory
- **2**: Low priority
- **1**: Temporary information, first to be cleaned up

### 3.2 Edge (Relationship)

An Edge connects two insights, representing their relationship. Each edge contains:

```
┌────────────────────────────────────────────┐
│ Edge                                       │
├────────────────────────────────────────────┤
│ source_id  : UUID  ──→  target_id : UUID   │
│ edge_type  : temporal | semantic |         │
│              causal   | entity             │
│ weight     : 0.0 ~ 1.0                    │
│ metadata   : {"sub_type": "backbone", ...} │
└────────────────────────────────────────────┘
```

The four edge types form the foundation of the MAGMA four-graph model, detailed in [Section 5](#5-magma-four-graph-model).

### 3.3 Database Schema

Each named store has its own SQLite file under `~/.mnemon/data/<store>/mnemon.db`, using WAL mode to support concurrent reads. The default store is `default`; additional stores can be created for data isolation (see [Store Management](../USAGE.md#store-management)).

```sql
-- Memory nodes
insights (
  id, content, category, importance,
  tags, entities, source,
  embedding,                    -- Optional, 768-dim vector
  access_count, last_accessed_at,
  effective_importance,          -- Decayed effective importance
  created_at, updated_at, deleted_at
)

-- Relationship edges (composite primary key)
edges (
  source_id, target_id, edge_type,  -- PK
  weight, metadata, created_at
)

-- Operation log (audit trail)
oplog (
  id, operation, insight_id, detail, created_at
)
```

---

## 4. System Architecture

Mnemon's architecture is divided into five layers:

```
┌─────────────────────────────────────────────────────────────┐
│  Integration Layer    Hook / Skill / Guide                   │
├─────────────────────────────────────────────────────────────┤
│  CLI Layer            remember, recall, diff, link, gc ...  │
├─────────────────────────────────────────────────────────────┤
│  Core Engine          search/ (recall, intent, keyword)     │
│                       graph/  (temporal, entity, causal,    │
│                                semantic)                    │
│                       embed/  (ollama, vector)              │
├─────────────────────────────────────────────────────────────┤
│  Storage Layer        store/  (db, node, edge, oplog)       │
├─────────────────────────────────────────────────────────────┤
│  External (Optional)  Ollama (localhost:11434)               │
└─────────────────────────────────────────────────────────────┘
```


**Project code structure:**

```
mnemon/
├── cmd/                       # CLI commands (Cobra)
│   ├── root.go                # Root command, global flags, store resolution
│   ├── store.go               # Store management (list, create, set, remove)
│   ├── remember.go            # Store insight + auto-create edges
│   ├── recall.go              # Retrieval (smart graph-enhanced, default)
│   ├── diff.go                # Standalone dedup/conflict check
│   ├── link.go                # Manually create edges
│   ├── related.go             # BFS traversal from an insight
│   ├── search.go              # Keyword search
│   ├── embed.go               # Manage embeddings
│   ├── forget.go              # Soft-delete insight
│   ├── gc.go                  # Garbage collection
│   ├── setup.go               # Deploy integrations (hooks, skill, guide)
│   ├── viz.go                 # Knowledge graph visualization
│   ├── status.go              # Statistics
│   └── log.go                 # Operation log
├── internal/
│   ├── model/                 # Data structures
│   │   ├── node.go            # Insight definition
│   │   └── edge.go            # Edge definition
│   ├── graph/                 # MAGMA four-graph implementation
│   │   ├── engine.go          # Auto edge-creation orchestrator
│   │   ├── temporal.go        # Temporal edges
│   │   ├── entity.go          # Entity edges
│   │   ├── causal.go          # Causal edges
│   │   └── semantic.go        # Semantic edges
│   ├── search/                # Retrieval algorithms
│   │   ├── recall.go          # Intent-aware multi-signal retrieval
│   │   ├── diff.go            # Built-in dedup check
│   │   ├── intent.go          # Intent detection
│   │   └── keyword.go         # Token-level keyword scoring
│   ├── store/                 # SQLite persistence
│   │   ├── db.go              # Database initialization, transactions, store management
│   │   ├── node.go            # Insight CRUD, lifecycle
│   │   ├── edge.go            # Edge CRUD
│   │   └── oplog.go           # Operation log
│   ├── embed/                 # Embedding support
│   │   ├── ollama.go          # Ollama HTTP client
│   │   └── vector.go          # Vector serialization, cosine similarity
│   └── setup/                 # LLM CLI integration setup
│       ├── claude.go          # Claude Code deployment logic
│       ├── openclaw.go        # OpenClaw deployment logic
│       ├── detect.go          # Environment detection
│       ├── prompt.go          # Prompt file deployment (guide.md)
│       ├── settings.go        # Hook registration in settings.json
│       ├── markdown.go        # Markdown injection/ejection
│       └── assets/            # Embedded templates (synced from source)
│           ├── claude/        # Claude Code assets
│           │   ├── SKILL.md, guide.md
│           │   ├── prime.sh, user_prompt.sh
│           │   ├── stop.sh, compact.sh
│           └── openclaw/      # OpenClaw assets
│               └── SKILL.md
├── scripts/
│   └── e2e_test.sh            # End-to-end test suite
├── main.go                    # Entry point
├── CLAUDE.md                  # Project-level development guidelines
└── Makefile                   # Build, install, test
```

### 4.1 Data Directory Layout

```
~/.mnemon/
├── active                        # Current default store name (plain text)
├── prompt/                       # Shared across all stores
│   ├── guide.md                  # Behavioral guide (recall/remember rules)
│   └── skill.md                  # Skill definition (command reference)
└── data/                         # Each store has its own isolated directory
    ├── default/
    │   └── mnemon.db             # SQLite database (WAL mode)
    ├── work/
    │   └── mnemon.db
    └── <name>/
        └── mnemon.db
```

**Isolation boundary**: Each store contains an independent `mnemon.db` — insights, edges, and oplog are fully isolated. Prompt files (`guide.md`, `skill.md`) are shared — behavioral rules are universal, memory data is private.

### 4.2 Store Isolation

Mnemon supports named stores for lightweight data isolation between different agents, projects, or scenarios.

**Why named stores instead of just `--data-dir`?**

`--data-dir` overrides the entire base directory — a blunt instrument that requires the caller to manage full paths. Named stores provide semantic clarity (`MNEMON_STORE=work` vs `--data-dir ~/.mnemon-work`) and work naturally with environment variables, which are the standard isolation mechanism for concurrent processes.

**Resolution priority** (highest to lowest):

```
--store flag  >  MNEMON_STORE env  >  ~/.mnemon/active file  >  "default"
```

This layered design serves different scenarios:

| Mechanism | Scenario |
|-----------|----------|
| `--store` flag | One-off CLI override, scripting |
| `MNEMON_STORE` env | Per-process isolation — different agents use different stores |
| `active` file | Persistent user preference — `mnemon store set work` |
| `"default"` | Zero-config — works out of the box |

**Automatic migration**: When the new `data/` directory doesn't exist but a legacy `~/.mnemon/mnemon.db` does, mnemon automatically moves it to `data/default/mnemon.db`. Users upgrading from older versions experience a seamless transition.

**Design principle — lightweight and bounded**: Store isolation addresses a necessary data separation concern without growing into a multi-tenant system. There are no access controls, no cross-store queries, no store metadata beyond the name. This keeps the feature bounded — Mnemon is a memory daemon, not a knowledge base platform.

---

## 5. MAGMA Four-Graph Model

The core idea of the MAGMA paper is: **a single edge type (such as pure vector similarity) is insufficient to capture the multidimensional relationships between memories.** Different query intents require different relational perspectives — asking "why" requires causal chains, asking "when" requires timelines, asking "about X" requires entity associations.

Mnemon implements four graphs, each capturing one dimension of relationships:

![MAGMA Four-Graph Model](diagrams/04-magma-four-graph.jpg)

### 5.1 Temporal Graph

**Purpose**: Capture the chronological order of memories, building a temporal skeleton of the knowledge flow.

**Automatically created edges**:

- **Backbone**: New insight → most recent insight from the same source (bidirectional)
  - Ensures memories from each source (user/agent) form a continuous timeline
- **Proximity**: New insight <-> insights within a 24-hour window (bidirectional)
  - Weight formula: `w = 1 / (1 + hours_diff)`
  - Up to 10 proximity edges

```
Insight A (2h ago) ←── backbone ──→ Insight B (1h ago) ←── backbone ──→ Insight C (now)
     ↑                                     ↑
     └──────── proximity (w=0.33) ─────────┘
```

**Metadata**: `{"sub_type": "backbone"|"proximity", "hours_diff": "2.34"}`

### 5.2 Entity Graph

**Purpose**: Link insights that mention the same entities.

**Entity extraction (hybrid approach)**:
1. **Regex patterns**: CamelCase (`HttpServer`), ALL_CAPS (`API`), file paths (`./cmd/root.go`), URLs, @-mentions, Chinese book title marks
2. **Technical dictionary**: 200+ common terms (Go, React, SQLite, Kubernetes...)
3. **User-provided**: `--entities` flag for direct specification

**Automatically created edges**: New insight <-> up to 5 existing insights per shared entity (bidirectional), weight 1.0.

```
                   ┌─── "Qdrant" ───┐
                   │                │
Insight A ←── entity ──→ Insight B ←── entity ──→ Insight C
("Chose Qdrant")         ("Qdrant perf test")     ("Qdrant deployment config")
```

**Metadata**: `{"entity": "Qdrant"}`

### 5.3 Causal Graph

**Purpose**: Capture the reasons behind decisions and cause-effect relationships.

**Automatic detection**:
1. Content contains causal keywords (`because`, `therefore`, `due to`, `caused by`, `as a result`, etc.)
2. Token overlap with recent insights >= 15%
3. Direction inference: causal direction is determined based on whether the causal keyword appears in the new or existing insight

**LLM-assisted evaluation**:
- `remember` outputs a causal candidate list (discovered via 2-hop BFS)
- The host LLM evaluates these candidates and decides whether to establish connections via `link --type causal`
- Supports sub-type hints: `causes` (direct cause), `enables` (enabling condition), `prevents` (preventing factor)

```
Insight A ──── causal ────→ Insight B
("Team lacks Redis exp.")   ("Chose SQLite as storage")
  sub_type: "causes"
  weight: 0.75
```

This is a quintessential example of the LLM-Supervised philosophy: Binary handles low-cost candidate discovery (regex + token overlap), while the LLM handles high-value causal judgment.

### 5.4 Semantic Graph

**Purpose**: Connect semantically similar insights based on meaning.

**Two-tier confidence system**:

| Tier | Cosine Similarity | Behavior |
|------|-------------------|----------|
| **Auto-link** | >= 0.80 | Automatically create bidirectional edges (high confidence), up to 3 |
| **Candidate review** | 0.40 ~ 0.79 | Output to LLM for evaluation; LLM decides whether to link |
| **Ignore** | < 0.40 | No action |

**Fallback** (without embeddings): Token overlap rate is used instead of cosine similarity.

```
Insight A ←── semantic (auto, cos=0.92) ──→ Insight B

Insight C ←── semantic (LLM review) ──→ Insight D
                cos=0.65, manually linked after LLM judged "related"
```

### 5.5 Four-Graph Synergy: Intent-Adaptive Weighting

Different query intents activate different graph traversal weights:

| Intent | Causal | Temporal | Entity | Semantic |
|--------|--------|----------|--------|----------|
| **WHY** | **0.70** | 0.20 | 0.05 | 0.05 |
| **WHEN** | 0.15 | **0.65** | 0.10 | 0.10 |
| **ENTITY** | 0.10 | 0.05 | **0.55** | 0.30 |
| **GENERAL** | 0.25 | 0.25 | 0.25 | 0.25 |

When asking "why was SQLite chosen," the causal edge weight is highest, so the system traces decision rationale along causal chains. When asking for "memories related to React," the entity edge weight is highest, so the system finds all insights mentioning React.

---

## 6. Write Pipeline: Remember

`mnemon remember` is the core command for writing memories. It includes a built-in diff step that automatically detects duplicates and conflicts before storage. The write transaction executes atomically within a single SQLite transaction.

![Remember Pipeline](diagrams/02-remember-pipeline.jpg)

### 6.1 Detailed Flow

```
mnemon remember "Chose Qdrant as the vector database" \
  --cat decision --imp 5 --entities "Qdrant,Milvus"
```

**Step 1: Validate Input**
- Category must be one of the six types
- Importance 1-5
- Content must not exceed 8000 characters
- Up to 20 tags and 50 entities

**Step 2: Generate Embedding (outside transaction)**
- If Ollama is available: HTTP POST -> nomic-embed-text -> 768-dim float64 vector
- If unavailable: embedding = nil, falls back to token overlap downstream

**Step 2.5: Built-in Diff (outside transaction, read-only)**

Compute similarity against all active insights:
- **DUPLICATE** (sim > 0.90) → skip insert entirely, return `action="skipped"`
- **CONFLICT/UPDATE** (sim 0.50–0.90) → soft-delete old insight, insert new as replacement
- **ADD** (sim < 0.50) → normal insert

This step uses embedding cosine similarity when available, falling back to token overlap. The `--no-diff` flag disables this check.

**Step 3: Atomic Transaction**

```
BEGIN TRANSACTION
  ⓪ Soft-delete replaced insight (if diff found CONFLICT/UPDATE)
  ① INSERT insight (UUID, content, category, importance, tags, entities, source)
  ② UPDATE embedding (if vector is available)
  ③ Graph Engine: OnInsightCreated
     ├── CreateTemporalEdge    → backbone + 24h proximity
     ├── CreateEntityEdges     → regex + dictionary extraction → co-occurrence links
     ├── CreateCausalEdges     → keywords + token overlap → auto causal edges
     └── CreateSemanticEdges   → cos >= 0.80 auto-link
  ④ RefreshEffectiveImportance → update EI decay values
  ⑤ AutoPrune                 → soft-delete lowest EI when total > 1000
COMMIT
```

**Step 4: Candidate Output (post-transaction, read-only)**
- `FindSemanticCandidates`: Semantic candidates with cos in [0.40, 0.80)
- `FindCausalCandidates`: Causal candidates in the 2-hop BFS neighborhood

**Step 5: JSON Output**

```json
{
  "id": "abc-123",
  "action": "added",
  "diff_suggestion": "ADD",
  "replaced_id": null,
  "edges_created": {"temporal": 2, "entity": 3, "causal": 1, "semantic": 1},
  "semantic_candidates": [
    {"id": "def-456", "content": "...", "cosine": 0.72, "auto_linked": false}
  ],
  "causal_candidates": [
    {"id": "ghi-789", "content": "...", "hop": 1, "suggested_sub_type": "causes"}
  ],
  "embedded": true,
  "effective_importance": 0.85,
  "auto_pruned": 0
}
```

The `action` field indicates what the built-in diff decided: `"added"` (new entry), `"replaced"` (conflict auto-replaced, `replaced_id` contains the old insight ID), or `"skipped"` (duplicate detected, no insert).

After receiving this output, the LLM can evaluate candidates and establish edges it considers appropriate via the `mnemon link` command.

---

## 7. Read Pipeline: Smart Recall (Default)

`mnemon recall` is Mnemon's core retrieval algorithm. Smart recall is the default mode for all queries. It combines intent detection, multi-signal anchor selection, Beam Search graph traversal, and multi-factor re-ranking to achieve intent-aware graph-enhanced retrieval. Use `--basic` for legacy SQL LIKE fallback.

![Smart Recall Pipeline](diagrams/03-smart-recall-pipeline.jpg)

### 7.1 Step 1: Intent Detection

Query intent is automatically identified via regex matching:

| Intent | Trigger Patterns |
|--------|-----------------|
| WHY | `why`, `reason`, `because`, `cause`, `motivation`, `为什么`, `原因`, `理由` |
| WHEN | `when`, `time`, `before`, `after`, `timeline`, `什么时候`, `何时`, `时间` |
| ENTITY | `what is`, `who is`, `tell me about`, `是什么`, `谁是`, `关于` |
| GENERAL | None of the above match |

Supports the `--intent` flag to manually override automatic detection.

### 7.2 Step 2: Multi-Signal Anchor Selection (RRF Fusion)

Multiple signals run in parallel and are merged via Reciprocal Rank Fusion:

```
Signal 1: Keyword     → KeywordSearch(all_insights, query, top-20)
Signal 2: Vector      → CosineSimilarity(query_vec, all_embeddings, top-20)
Signal 3: Recency     → sort by created_at DESC, top-20
Signal 4: Entity      → insights sharing entities with the query

RRF Score = Σ  1 / (k + rank_i + 1)    (k = 60)
                 for each signal
```

Each insight may rank differently across signals; RRF fusion produces a robust composite ranking.

### 7.3 Step 3: Beam Search Graph Traversal

Starting from each anchor, Beam Search is performed across the four graphs:

```
for each anchor:
    priority_queue = [(anchor, initial_score)]
    visited = {}

    while budget_remaining:
        node = pop(priority_queue)
        for edge in GetEdgesFrom(node):
            neighbor = edge.target
            structural_score = edge.weight × intent_weight[edge.type]
            semantic_score = cosine(vec_neighbor, vec_query)
            total = score_node + λ₁·structural + λ₂·semantic
            //  λ₁ = 1.0 (structural weight), λ₂ = 0.4 (semantic weight)

            if total > best_score[neighbor]:
                update(neighbor, total)
                push(priority_queue, neighbor)
```

**Adaptive parameters**:

| Intent | Beam Width | Max Depth | Max Visited |
|--------|-----------|-----------|-------------|
| WHY | 15 | 5 | 500 |
| WHEN | 10 | 5 | 400 |
| ENTITY | 10 | 4 | 400 |
| GENERAL | 10 | 4 | 500 |

WHY queries use a wider beam and deeper traversal because causal chains typically span multiple hops.

### 7.4 Step 4: Multi-Factor Re-Ranking

For all collected candidates, a four-dimensional score is computed and combined via weighted sum:

```
keyword_score  = token_intersection / query_token_count
entity_score   = matched_entities / max(1, query_entities_count)
similarity     = cosine(vec_candidate, vec_query)
graph_score    = (traversal_score - min) / (max - min)   // min-max normalization

final = w_kw·keyword + w_ent·entity + w_sim·similarity + w_gr·graph
```

Weights vary by intent:

| Intent | Keyword | Entity | Similarity | Graph |
|--------|---------|--------|------------|-------|
| WHY | 0.10 | 0.10 | 0.30 | **0.50** |
| WHEN | 0.15 | 0.15 | 0.30 | **0.40** |
| ENTITY | 0.20 | **0.40** | 0.20 | 0.20 |
| GENERAL | 0.25 | 0.25 | 0.25 | 0.25 |

### 7.5 Step 5: WHY Post-Processing — Causal Topological Sort

If the intent is WHY, an additional topological sort using Kahn's algorithm is performed: results are arranged along causal edges so that **causes come first, effects follow**.

### 7.6 Signal Transparency

Each retrieval result includes a detailed signal breakdown:

```json
{
  "insight": {"id": "...", "content": "..."},
  "score": 0.73,
  "intent": "ENTITY",
  "via": "keyword",
  "signals": {
    "keyword": 0.85,
    "entity": 0.60,
    "similarity": 0.72,
    "graph": 0.45
  }
}
```

This is a unique innovation in Mnemon: **exposing the retrieval pipeline's internal signals to the host LLM**. Since the host LLM has the full conversation context, it can make better re-ranking judgments than any algorithm inside the pipeline.

---

## 8. Deduplication & Conflict Detection: Diff

![Diff & Dedup Pipeline](diagrams/07-diff-dedup-pipeline.jpg)

Diff is **built into `remember`** — no separate call needed. When `mnemon remember` is invoked, it automatically runs a diff check before inserting.

When `remember` is called, the built-in diff runs before the transaction:

1. Compute similarity against all active insights (embedding cosine when available, token overlap as fallback)
2. Determine the action based on similarity thresholds:

| Similarity | Action | Behavior |
|------------|--------|----------|
| > 0.90 | **DUPLICATE** | Skip insert entirely, return `action="skipped"` |
| 0.50 ~ 0.90 | **CONFLICT/UPDATE** | Soft-delete old insight, insert new as replacement |
| < 0.50 | **ADD** | Normal insert |

The `--no-diff` flag disables this check for cases where the caller wants unconditional insertion.

### 8.1 Typical Workflow

A single `remember` call handles everything:

```bash
# Single command — diff is automatic
mnemon remember "Chose PostgreSQL to replace SQLite as the primary database" \
  --cat decision --imp 5 --source agent
# → If conflict with existing "Chose SQLite as storage":
#   auto-replaces old insight, returns action="replaced", replaced_id="<old_id>"
# → If duplicate: returns action="skipped"
# → If new: returns action="added"
```

---

## 9. Lifecycle Management

Mnemon is not an append-only system. Effective memory management requires important memories to persist while outdated ones naturally decay.

![Lifecycle & Retention](diagrams/06-lifecycle-retention.jpg)

### 9.1 Effective Importance (EI)

EI combines base importance, access frequency, time decay, and graph connectivity:

```
EI = base_weight(importance) × access_factor × decay_factor × edge_factor

base_weight:   imp 5 → 1.0,  4 → 0.8,  3 → 0.5,  2 → 0.3,  1 → 0.15
access_factor: max(1.0, log(1 + access_count))
decay_factor:  0.5 ^ (days_since_access / 30)     // half-life of 30 days
edge_factor:   1.0 + 0.1 × min(edge_count, 5)     // up to +0.5
```

Interpretation:
- **High importance** -> higher base score
- **Frequent access** -> logarithmic growth bonus
- **Long period without access** -> exponential decay (halves every 30 days)
- **Rich graph connections** -> indicates relevance to other knowledge, bonus applied

### 9.2 Immunity Rules

The following insights are exempt from automatic cleanup:
- `importance >= 4` (high-value memories)
- `access_count >= 3` (frequently retrieved)

### 9.3 Auto-Pruning

Triggered when the total number of active insights exceeds **1000**:

1. Compute EI for all insights
2. Exclude immune insights
3. Take the lowest EI entries in ascending order (up to 10 per batch)
4. Soft-delete (set `deleted_at`)
5. Cascade-delete related edges

### 9.4 GC Command

Manual lifecycle management tool:

```bash
# View low-retention candidates
mnemon gc --threshold 0.5

# Retain a specific insight (increases access_count by +3)
mnemon gc --keep <id>
```

---

## 10. Embedding Support

Embedding vectors are an optional enhancement. Without embeddings, Mnemon operates entirely on keywords and graph structure; with embeddings, semantic retrieval capabilities are significantly enhanced.

### 10.1 Ollama Integration

Via the local Ollama service (no external API required):

```
Mnemon ──HTTP──→ Ollama (localhost:11434)
                  └── nomic-embed-text
                      768-dim vector
```

- **Availability detection**: 2-second timeout to avoid blocking
- **Graceful degradation**: Automatically falls back to token overlap when Ollama is unavailable
- **Zero new dependencies**: Pure stdlib `net/http`

### 10.2 Vector Storage

Vectors are serialized as little-endian float64 BLOBs stored in the `insights.embedding` column (768 x 8 = 6144 bytes/insight).

### 10.3 Usage Scenarios

| Scenario | Without Embedding | With Embedding |
|----------|------------------|----------------|
| remember -> semantic edges | Token overlap > 0.10 | cos >= 0.80 auto-link |
| recall -> anchors | Keyword + recency | Keyword + vector + recency |
| recall -> traversal | Pure structural score | Structural + semantic similarity |
| recall -> re-ranking | KW + Entity + Graph | KW + Entity + Similarity + Graph |

### 10.4 Management Commands

```bash
ollama pull nomic-embed-text    # Install the model
mnemon embed --status           # View coverage
mnemon embed --all              # Batch-generate embeddings for all insights
mnemon embed <id>               # Generate for a single insight
```

---

## 11. LLM CLI Integration

![Integration Architecture](diagrams/08-three-layer-integration.jpg)

Mnemon integrates with LLM CLIs through lifecycle hooks, a skill file, and a behavioral guide. Claude Code's [hook system](https://docs.anthropic.com/en/docs/claude-code/hooks) is the reference implementation — all components are deployed automatically via `mnemon setup`.

### 11.1 Integration Architecture

Four hooks drive the memory lifecycle:

```
Session starts
    │
    ▼
  Prime (SessionStart) ─── prime.sh ──→ load guide.md (memory execution manual)
    │
    ▼
  User sends message
    │
    ▼
  Remind (UserPromptSubmit) ─── user_prompt.sh ──→ remind agent to recall & remember
    │
    ▼
  Skill (SKILL.md) ── command syntax reference (auto-discovered)
    │
    ▼
  LLM generates response (following guide.md behavioral rules)
    │
    ▼
  Nudge (Stop) ─── stop.sh ──→ remind agent to remember
    │
    ▼
  (when context compacts)
  Compact (PreCompact) ─── compact.sh ──→ extract critical insights to remember
```

Three layers work together:

| Layer | What | Where | Role |
|-------|------|-------|------|
| **Hooks** | Shell scripts triggered by Claude Code lifecycle events | `.claude/hooks/mnemon/` | Prime (guide), Remind (recall & remember), Nudge (remember), Compact (critical save) |
| **Skill** | `SKILL.md` — command reference in Claude Code skill format | `.claude/skills/mnemon/` | Teaches the LLM *how* to use mnemon commands |
| **Guide** | `guide.md` — detailed execution manual for recall, remember, and delegation | `~/.mnemon/prompt/` | Teaches the LLM *when* to recall, *what* to remember, and *how* to delegate |

### 11.2 Hook Details

Claude Code fires hooks at specific lifecycle events. Mnemon registers up to four, each with a distinct role in the memory lifecycle:

**Prime (SessionStart) — `prime.sh`**

Runs once when a session starts. Loads the behavioral guide — a detailed execution manual that teaches the agent when to recall, what to remember, and how to delegate memory writes:

```bash
STATS=$(mnemon status 2>/dev/null)
if [ -n "$STATS" ]; then
  # extract counts from JSON and show in status line
  echo "[mnemon] Memory active (<insights> insights, <edges> edges)."
else
  echo "[mnemon] Memory active."
fi
[ -f ~/.mnemon/prompt/guide.md ] && cat ~/.mnemon/prompt/guide.md
```

The guide content appears in the LLM's system context, establishing recall/remember/delegation behavior for the entire session.

**Remind (UserPromptSubmit) — `user_prompt.sh`**

Runs on every user message. A lightweight prompt that reminds the agent to evaluate whether recall and remember are needed before starting work:

```bash
echo "[mnemon] Evaluate: recall needed? After responding, evaluate: remember needed?"
```

The agent decides whether to act on this reminder based on the guide.md rules — it is a suggestion, not forced execution.

**Nudge (Stop) — `stop.sh`**

Runs after each LLM response. Reminds the agent to consider whether the exchange warrants a remember operation. Stays silent if memory was already addressed:

```bash
MSG=$(echo "$INPUT" | jq -r '.last_assistant_message // ""' 2>/dev/null)
if echo "$MSG" | grep -qi "mnemon remember\|sub-agent.*remember\|Stored.*imp="; then
  exit 0  # Already handled
fi
echo "[mnemon] Consider: does this exchange warrant a remember sub-agent?"
```

**Compact (PreCompact) — `compact.sh` (optional)**

Fires before context window compression. Instructs the agent to extract the most critical insights and remember them before context is lost:

```bash
echo "[mnemon] Context compaction starting. Review this session and remember the most valuable insights (up to 5) before context is compressed. Delegate to Task sub-agents now."
```

### 11.3 Automated Setup

`mnemon setup` handles all deployment automatically:

```
$ mnemon setup

Detecting LLM CLI environments...
  ✓ Claude Code (v1.x)    .claude/

Select environment: Claude Code
Install scope: Local — this project only (.claude/)

[1/3] Skill
  ✓ Skill     .claude/skills/mnemon/SKILL.md

[2/3] Prompts
  ✓ Prompts   ~/.mnemon/prompt/ (guide.md, skill.md)

[3/3] Optional hooks
  Select hooks to enable:
    [x] Remind  — remind agent to recall & remember (recommended)
    [x] Nudge   — remind agent to remember after work
    [ ] Compact — extract critical insights before compaction

Setup complete!
  Hooks   prime, remind, nudge
  Prompts ~/.mnemon/prompt/ (guide.md, skill.md)

Start a new Claude Code session to activate.
Edit ~/.mnemon/prompt/guide.md to customize behavior.
Run 'mnemon setup --eject' to remove.
```

Key setup options:

| Flag | Effect |
|------|--------|
| `--global` | Install to `~/.claude/` (all projects) instead of `.claude/` (project-local) |
| `--target claude-code` | Non-interactive, Claude Code only |
| `--eject` | Remove all mnemon integrations |
| `--yes` | Auto-confirm all prompts (CI-friendly) |

The Prime hook is always installed. Remind, Nudge, and Compact hooks are optional (Remind and Nudge enabled by default).

### 11.4 Sub-Agent Delegation

Memory writes don't happen in the main conversation. Instead, the host LLM delegates to a lightweight sub-agent:

```
Main Agent (Opus)                     Sub-Agent (Sonnet)
┌──────────────────────┐              ┌──────────────────────┐
│ Full conversation     │  delegates   │ ~1000 tokens context │
│ context (~25k tokens) │ ──────────→ │ Reads SKILL.md       │
│                       │              │ Executes commands    │
│ Decides WHAT to       │  result      │ Evaluates candidates │
│ remember              │ ←────────── │ with judgment        │
└──────────────────────┘              └──────────────────────┘
```

**Why sub-agent?**

| Dimension | Main conversation | Sub-agent |
|-----------|-------------------|-----------|
| Context size | ~25,000 tokens | ~1,000 tokens |
| Model | Opus (expensive) | Sonnet (cheaper) |
| Scope | Full conversation | Memory task only |
| Execution | Synchronous, blocks user | Background, non-blocking |

The main agent provides only WHAT to store — content, category, importance, entities. The sub-agent reads SKILL.md, executes the correct `mnemon remember` command, and evaluates `remember`'s link candidates with judgment — not mechanical rules.

This separation means:

- **Token economy**: ~7,000 total tokens per memory write vs ~25,000 if done in main conversation
- **Context isolation**: Memory processing doesn't pollute the main conversation context
- **Model efficiency**: Sonnet handles routine execution while Opus focuses on high-level decisions

### 11.5 Adapting to Other LLM CLIs

For CLIs with hook support, replicate the Claude Code pattern: register lifecycle hooks that call mnemon commands, deploy the skill file, and provide the behavioral guide.

For CLIs without hook support, merge the recall/remember guidance into the corresponding system prompt file:

- Cursor -> `.cursorrules`
- Windsurf -> `RULES.md`
- OpenClaw -> `mnemon setup --target openclaw` deploys skill + guide, but hooks require manual plugin configuration
- Others -> System prompt / rules file

---

## 12. Design Decisions & Trade-offs

### Why LLM-Supervised Instead of an Embedded LLM?

| Dimension | LLM-Embedded (Mem0, etc.) | LLM-Supervised (Mnemon) |
|-----------|--------------------------|--------------------------|
| LLM Capability | gpt-4o-mini (constrained) | Host LLM (Opus-class) |
| API Cost | Every operation incurs a call | Zero |
| Network Dependency | Required | Not required |
| Swappability | API-bound | Any LLM CLI |

### Why SQLite WAL Instead of an Embedded Graph Database?

- **Single-file deployment**: one `.db` file per store — easy to manage and backup
- **ACID transactions**: Atomicity guarantee for the remember pipeline
- **WAL concurrency**: Supports simultaneous hook reads and CLI writes
- **Zero external dependencies**: No Redis/Neo4j/Qdrant required
- **Store isolation**: Named stores (`~/.mnemon/data/<name>/mnemon.db`) provide lightweight data isolation via `MNEMON_STORE` env var

### Why Beam Search Instead of Full BFS?

- **Budget control**: MaxVisited parameter prevents graph explosion
- **Intent-adaptive**: Different intents use different beam widths and depths
- **Quality assurance**: Only the highest-scoring candidates are retained at each level, similar to pruning

### Why Soft Delete?

- Preserves audit trail
- Supports "undo" (recovering accidental deletions)
- Simplifies cascade cleanup
- Query consistency (`WHERE deleted_at IS NULL`)

### Key Deviations from the MAGMA Paper

| Aspect | MAGMA Paper | Mnemon Implementation |
|--------|------------|----------------------|
| Entity Extraction | LLM-driven full pipeline | Regex + dictionary + LLM supplementation |
| Causal Reasoning | Embedded prompt chain | Auto candidates + LLM review |
| Node Types | EVENT, EPISODE, SESSION, NARRATIVE | Insight only (flat) |
| Storage | NetworkX (in-memory) | SQLite (persistent) |
| Embeddings | FAISS + OpenAI | Ollama (local, optional) |
| Deployment | Python library | Single Go binary |

Mnemon retains MAGMA's **architectural skeleton** (four-graph separation, intent-adaptive retrieval, multi-signal fusion) while replacing academic implementation details with production-ready simplifications. The core trade-off is: **use regex/heuristics to handle 80% of automation scenarios, and delegate the 20% requiring deep understanding to the host LLM.**
