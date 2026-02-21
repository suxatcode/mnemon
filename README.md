<p align="center">
  <img src="docs/logo/logo.svg" width="160" height="160" alt="Mnemon Logo" />
</p>

# Mnemon

**LLM-supervised persistent memory for AI agents.**

[![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![CI](https://github.com/mnemon-dev/mnemon/actions/workflows/ci.yml/badge.svg)](https://github.com/mnemon-dev/mnemon/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/mnemon-dev/mnemon)](https://goreportcard.com/report/github.com/mnemon-dev/mnemon)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

---

LLM agents forget everything between sessions. Context compaction drops critical decisions, cross-session knowledge vanishes, and long conversations push early information out of the window.

Mnemon gives your agent persistent, cross-session memory — a four-graph knowledge store with intent-aware recall, importance decay, and automatic deduplication. Single binary, zero API keys, one setup command.

### Why Mnemon?

Most memory tools embed their own LLM inside the pipeline. Mnemon takes a different approach: **your host LLM is the supervisor.** The binary handles deterministic computation (storage, graph indexing, search, decay); the LLM makes judgment calls (what to remember, how to link, when to forget). No middleman, no extra inference cost.

| Pattern | LLM Role | Representative |
|---|---|---|
| **LLM-Embedded** | Executor inside the pipeline | Mem0, Letta |
| **MCP Server** | Tool provider via MCP protocol | claude-mem |
| **LLM-Supervised** | External supervisor of a standalone binary | **Mnemon** |

<p align="center">
  <img src="docs/diagrams/llm-supervised-concept.jpg" width="720" alt="LLM-Supervised Architecture — three patterns compared, with detailed Mnemon implementation showing hooks, brain/organ split, and sub-agent delegation" />
  <br />
  <sub>The LLM-Supervised pattern: hooks drive the lifecycle, the host LLM makes judgment calls, the binary handles deterministic computation.</sub>
</p>

Memory has a **compound interest effect** — the longer it accumulates, the greater its value. LLM engines iterate constantly, skill files cost nearly nothing to write, but memory is a private asset that grows with the user. It is the only component in the agent ecosystem worth deep investment.

<p align="center">
  <img src="docs/diagrams/10-knowledge-graph.jpg" width="720" alt="Knowledge Graph — 87 insights connected by temporal, entity, semantic, and causal edges" />
  <br />
  <sub>A real knowledge graph built by Mnemon — 87 insights, 2150 edges across four graph types.</sub>
</p>

See [Design & Architecture](docs/DESIGN.md) for details.

## Quick Start

### Install

**Homebrew** (macOS / Linux):

```bash
brew install mnemon-dev/tap/mnemon
```

**Go install**:

```bash
go install github.com/mnemon-dev/mnemon@latest
```

**From source**:

```bash
git clone https://github.com/mnemon-dev/mnemon.git && cd mnemon
make install
```

**Verify installation**:

```bash
mnemon --version
```

### Claude Code

```bash
mnemon setup
```

`mnemon setup` auto-detects Claude Code, then interactively deploys skill, hooks, and behavioral guide. Start a new session — memory just works.

### [OpenClaw](https://github.com/openclaw/openclaw)

```bash
mnemon setup --target openclaw
```

This installs the skill and deploys a behavioral guide to `~/.mnemon/prompt/guide.md`. Since hook integration is not yet automated for OpenClaw, provide the guide to your agent and let it self-configure:

> Read `~/.mnemon/prompt/guide.md` and configure yourself to follow its recall/remember workflow.

### Uninstall

```bash
mnemon setup --eject
```

## How it works

Once set up, memory operates transparently — you use your LLM CLI as usual. Mnemon integrates via Claude Code's [hook system](https://docs.anthropic.com/en/docs/claude-code/hooks), injecting memory operations at key lifecycle points:

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
  LLM generates response (guided by skill + guide.md rules)
    │
    ▼
  Nudge (Stop) ─── stop.sh ──→ remind agent to remember
    │
    ▼
  (when context compacts)
  Compact (PreCompact) ─── compact.sh ──→ extract critical insights to remember
```

Four hooks drive the memory lifecycle. **Prime** loads the behavioral guide — a detailed execution manual for recall, remember, and sub-agent delegation. **Remind** prompts the agent to evaluate recall and remember before starting work. **Nudge** reminds the agent to consider remember after finishing work. **Compact** instructs the agent to extract and save critical insights before context compression. **The skill file** teaches command syntax. **The guide** (`~/.mnemon/prompt/guide.md`) defines the detailed rules for when to recall, what to remember, and how to delegate.

You don't run mnemon commands yourself. The agent does — driven by hooks and guided by the skill and behavioral guide.

## Features

- **Zero user-side operation** — install once, memory runs in the background via hooks
- **LLM-supervised** — the host LLM decides what to remember, update, and forget; no embedded LLM, no API keys
- **Hook-based integration** — four lifecycle hooks: Prime (load guide), Remind (recall & remember), Nudge (remember), and Compact (save before compression)
- **Four-graph architecture** — temporal, entity, causal, and semantic edges, not just vector similarity
- **Intent-aware recall** — graph traversal + optional vector search (RRF fusion), enabled by default for all queries
- **Built-in deduplication** — `remember` auto-detects duplicates and conflicts; skips or auto-replaces
- **Retention lifecycle** — importance decay, access-count boosting, and garbage collection
- **Optional embeddings** — works fully without Ollama; add local [Ollama](https://ollama.ai) for enhanced vector+keyword hybrid search

## Vision

All your local agentic AIs — across sessions and frameworks — sharing one pool of live memory.

```
  Claude Code ──┐
                │
  OpenClaw ─────┤
                ├──▶  ~/.mnemon  ◀── shared memory
  OpenCode ─────┤
                │
  Gemini CLI ───┘
```

The foundation is in place: a single `~/.mnemon` database that any agent can read and write. Claude Code's hook integration is the reference implementation — the same pattern (lifecycle hooks + skill + behavioral guide) can be replicated for any LLM CLI that supports event hooks or system prompts.

## FAQ

**Do different sessions share memory?**
Yes. By default, all sessions use the same `default` store — a decision remembered in one session is available in every future session.

**Can I isolate memory per project or agent?**
Yes. Use named stores to separate memory:

```bash
mnemon store create work        # create a new store
mnemon store set work           # set as default
MNEMON_STORE=work mnemon recall "query"  # or use env var per-process
```

Different agents/processes can use different stores via the `MNEMON_STORE` environment variable — no global state contention.

**Local or global mode?**
`mnemon setup` defaults to **local** (project-scoped `.claude/`), recommended for most users. **Global** (`mnemon setup --global`, installed to `~/.claude/`) activates mnemon across all projects — convenient if you want other frameworks (e.g., OpenClaw) to share memory by forwarding requests through Claude Code CLI, but may add maintenance overhead.

**How do I customize the behavior?**
Edit `~/.mnemon/prompt/guide.md`. This file controls when the agent recalls memories and what it considers worth remembering. The skill file (`SKILL.md`) is auto-deployed and should not need manual editing.

**What is sub-agent delegation?**
Memory writes don't happen in the main conversation. The host LLM (e.g., Opus) decides *what* to remember, then delegates the actual `mnemon remember` execution to a lightweight sub-agent (e.g., Sonnet). This saves tokens and keeps memory operations out of the main context.

## Configuration

| Environment Variable | Default | Description |
|---|---|---|
| `MNEMON_DATA_DIR` | `~/.mnemon` | Base data directory |
| `MNEMON_STORE` | *(active file or `default`)* | Named memory store for data isolation |

**Ollama-specific** (only relevant if using embeddings):

| Environment Variable | Default | Description |
|---|---|---|
| `MNEMON_EMBED_ENDPOINT` | `http://localhost:11434` | Ollama API endpoint |
| `MNEMON_EMBED_MODEL` | `nomic-embed-text` | Embedding model name |

## Development

```bash
make build          # build binary
make install        # build + install to $GOBIN
make test           # run E2E test suite
mnemon setup        # interactive setup
mnemon setup --eject  # remove all integrations
make help           # show all targets
```

**Dependencies**: Go 1.24+, `modernc.org/sqlite`, `spf13/cobra`, `google/uuid`

## Documentation

- [Design & Architecture](docs/DESIGN.md) — philosophy, algorithms, integration design
- [Usage & Reference](docs/USAGE.md) — CLI commands, embedding support, architecture overview
- [Architecture Diagrams](docs/diagrams/) — system architecture, pipelines, lifecycle management
- [中文文档](docs/zh/) — Chinese documentation

## References

Mnemon combines the paradigm of one paper with the methodology of another. See [Theoretical Foundations](docs/DESIGN.md#24-theoretical-foundations) for details.

- **RLM** — Zhang, Kraska & Khattab. [Recursive Language Models](https://arxiv.org/abs/2512.24601). 2025. Establishes the paradigm: LLMs are more effective as orchestrators of external environments than as direct data processors.
- **MAGMA** — Zou et al. [A Multi-Graph based Agentic Memory Architecture](https://arxiv.org/abs/2601.03236). 2025. Provides the methodology: four-graph model (temporal, entity, causal, semantic) with intent-adaptive retrieval.

## License

[MIT](LICENSE)
