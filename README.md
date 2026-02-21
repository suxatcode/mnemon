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

`mnemon setup` deploys skill, hooks, and behavioral guide automatically. Start a new session — memory just works.

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

Once set up, memory operates transparently — you use your LLM CLI as usual:

1. **You send a message** — the agent automatically recalls relevant memories from past sessions
2. **The agent responds** — drawing on both current context and recalled knowledge
3. **After responding** — the agent evaluates whether anything from this exchange is worth remembering for the future

You don't run mnemon commands yourself. The agent does — guided by the skill file (command reference) and the behavioral guide (when to recall, what to remember).

## Features

- **Zero user-side operation** — install once, memory runs in the background
- **LLM-supervised** — the host LLM decides what to remember, update, and forget; no embedded LLM, no API keys
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

The foundation is in place: a single `~/.mnemon` database that any agent can read and write.

## FAQ

**Do different sessions share memory?**
Yes. All sessions use the same `~/.mnemon` database — a decision remembered in one session is available in every future session.

**Local or global mode?**
`mnemon setup` defaults to **local** (project-scoped `.claude/`), recommended for most users. **Global** (`mnemon setup --global`, installed to `~/.claude/`) activates mnemon across all projects — convenient if you want other frameworks (e.g., OpenClaw) to share memory by forwarding requests through Claude Code CLI, but may add maintenance overhead.

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
