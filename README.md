<p align="center">
  <img src="docs/logo/logo.svg" width="160" height="160" alt="Mnemon Logo" />
</p>

# Mnemon

**Persistent memory for LLM agents.**

[![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![CI](https://github.com/mnemon-dev/mnemon/actions/workflows/ci.yml/badge.svg)](https://github.com/mnemon-dev/mnemon/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

---

LLM agents forget everything between sessions. Context compaction drops critical decisions, cross-session knowledge vanishes, and long conversations push early information out of the window.

Mnemon gives your agent persistent, cross-session memory — with a single binary and one setup command.

## Quick Start

### Claude Code

```bash
go install github.com/mnemon-dev/mnemon@latest
mnemon setup
```

`mnemon setup` auto-detects Claude Code, then interactively deploys skill, hooks, and behavioral guide. Start a new session — memory just works.

### OpenClaw

```bash
go install github.com/mnemon-dev/mnemon@latest
mnemon setup --target openclaw
```

This installs the skill and deploys a behavioral guide to `~/.mnemon/prompt/guide.md`. Since hook integration is not yet automated for OpenClaw, provide the guide to your agent and let it self-configure:

> Read `~/.mnemon/prompt/guide.md` and configure yourself to follow its recall/remember workflow.

### From source

```bash
git clone https://github.com/mnemon-dev/mnemon.git && cd mnemon
make install && mnemon setup
```

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
  SessionStart hook ─── prime.sh ──→ load behavioral guide, show memory status
    │
    ▼
  User sends message
    │
    ▼
  UserPromptSubmit hook ─── user_prompt.sh ──→ auto-recall relevant memories
    │
    ▼
  LLM generates response (guided by skill + behavioral rules)
    │
    ▼
  Stop hook ─── stop.sh ──→ "Consider: remember sub-agent?"
```

**Hooks** handle the plumbing — auto-recall on every message, memory reminders after each response, behavioral guide injection at session start. **The skill file** teaches the agent command syntax. **The behavioral guide** (`~/.mnemon/prompt/guide.md`) defines when to recall, what to remember, and how to delegate memory writes to a sub-agent.

You don't run mnemon commands yourself. The agent does — driven by hooks and guided by the skill and behavioral guide.

## Features

- **Zero user-side operation** — install once, memory runs in the background via hooks
- **LLM-supervised** — the host LLM decides what to remember, update, and forget; no embedded LLM, no API keys
- **Hook-based integration** — `mnemon setup` deploys lifecycle hooks (SessionStart, UserPromptSubmit, Stop) plus an optional Compact hook for context compression
- **Four-graph architecture** — temporal, entity, causal, and semantic edges, not just vector similarity
- **Built-in deduplication** — duplicates are skipped, conflicts auto-replaced
- **Retention lifecycle** — importance decay, access-count boosting, and garbage collection
- **Optional embeddings** — local [Ollama](https://ollama.ai) with `nomic-embed-text` for hybrid vector+keyword search

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
Yes. All sessions use the same `~/.mnemon` database — a decision remembered in one session is available in every future session.

**Local or global mode?**
`mnemon setup` defaults to **local** (project-scoped `.claude/`), recommended for most users. **Global** (`mnemon setup --global`, installed to `~/.claude/`) activates mnemon across all projects — convenient if you want other frameworks (e.g., OpenClaw) to share memory by forwarding requests through Claude Code CLI, but may add maintenance overhead.

**How do I customize the behavior?**
Edit `~/.mnemon/prompt/guide.md`. This file controls when the agent recalls memories and what it considers worth remembering. The skill file (`SKILL.md`) is auto-deployed and should not need manual editing.

**What is sub-agent delegation?**
Memory writes don't happen in the main conversation. The host LLM (e.g., Opus) decides *what* to remember, then delegates the actual `mnemon remember` execution to a lightweight sub-agent (e.g., Sonnet). This saves tokens and keeps memory operations out of the main context.

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `MNEMON_DATA_DIR` | `~/.mnemon` | Database directory |
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

- [Design & Architecture](docs/DESIGN.md) — philosophy, MAGMA four-graph model, algorithms, integration design
- [Architecture Diagrams](docs/diagrams/) — system architecture, pipelines, lifecycle management
- [中文文档](docs/zh/) — Chinese documentation

## License

[MIT](LICENSE)
