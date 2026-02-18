# Multi-CLI Integration Strategy

## Problem

Mnemon's three-layer integration (Hook + CLAUDE.md + Skill) is tightly coupled to Claude Code. Other LLM CLIs have different extension mechanisms, limiting portability.

## CLI Landscape

| CLI | Rules File | Hooks | Custom Commands | MCP | Sub-Agent |
|-----|-----------|:-----:|:---------------:|:---:|:---------:|
| Claude Code | `CLAUDE.md` | Full | Skills `.md` | Yes | Yes (mature) |
| Gemini CLI | `GEMINI.md` | Full (10+ events) | `.toml` commands | Yes | Yes (experimental) |
| OpenCode | `AGENTS.md` (+ `CLAUDE.md` fallback) | Full (25+ events, JS/TS plugins) | `.md` commands | Yes | Yes (Task tool) |
| Codex CLI | `AGENTS.md` | Notify only | Skills `SKILL.md` | Yes | Yes (spawn_agent) |
| Cursor | `.cursor/rules/*.mdc` | Full (6 events) | `.md` commands | Yes | Yes (recursive) |
| Qwen Code | `QWEN.md` | None documented | `.toml` commands | Yes | Unreliable |
| Aider | `CONVENTIONS.md` (manual) | None | None | No | No |

## Three-Layer Mapping

Current Claude Code layers and their equivalents:

### Layer 1: Auto-Recall (Hook)

| CLI | Mechanism | Event |
|-----|-----------|-------|
| Claude Code | `settings.json` hooks | `UserPromptSubmit` |
| Gemini CLI | `.gemini/settings.json` hooks | `BeforeAgent` |
| OpenCode | `.opencode/plugins/` JS module | `chat.message` |
| Cursor | `.cursor/hooks.json` | `beforeSubmitPrompt` |
| Codex CLI | N/A (no usable hook) | — |
| Qwen Code | N/A | — |
| Aider | N/A | — |

**Fallback for hookless CLIs**: Use rules file to instruct LLM to manually run `mnemon recall` at conversation start. Or use sub-agent to handle recall.

### Layer 2: Behavioral Guidance (Rules File)

Each CLI reads a different file, but the content is identical — just the filename changes.

### Layer 3: Command Reference (Skill/Command)

| CLI | Format | Location |
|-----|--------|----------|
| Claude Code | Markdown | `~/.claude/skills/mnemon/SKILL.md` |
| Codex CLI | Markdown with frontmatter | `.agents/skills/mnemon/SKILL.md` |
| Gemini CLI | TOML | `.gemini/commands/mnemon.toml` |
| OpenCode | Markdown | `.opencode/commands/mnemon.md` |
| Cursor | Markdown | `.cursor/commands/mnemon.md` |
| Qwen Code | TOML | `.qwen/commands/mnemon.toml` |

## Integration Tiers

### Tier 1: Full 3-layer (Hook + Rules + Command)

Claude Code, Gemini CLI, OpenCode, Cursor — all have hooks for auto-recall.

### Tier 2: 2-layer (Rules + Command, no auto-recall)

Codex CLI, Qwen Code — sub-agent can compensate for missing hooks.

### Tier 3: Single-layer (merged rules file)

Aider — all three layers merged into one `CONVENTIONS.md`.

## Proposed Implementation

```
scripts/
  adapters/
    claude/       # current implementation
    gemini/       # .gemini/ hooks + GEMINI.md + commands
    opencode/     # .opencode/ plugins + AGENTS.md + commands
    codex/        # .agents/skills/ + AGENTS.md
    cursor/       # .cursor/ hooks + rules + commands
    universal/    # merged single-file for hookless CLIs
```

```makefile
setup-gemini:    install inject-gemini inject-hooks-gemini
setup-opencode:  install inject-opencode inject-hooks-opencode
setup-cursor:    install inject-cursor inject-hooks-cursor
setup-codex:     install inject-codex        # no hooks
setup-universal: install inject-universal     # no hooks
```

Core binary unchanged — adapters only handle plumbing (file paths, formats, hook registration).

## Priority

1. **Gemini CLI** — full hook support, large user base, clean extension model
2. **Codex CLI** — `AGENTS.md` convention aligning with OpenAI ecosystem, skills system close to Mnemon's
3. **OpenCode** — reads `CLAUDE.md` fallback, minimal effort for basic support
4. **Cursor** — IDE market, different user profile

## Related

- [01-sub-agent-memory.md](01-sub-agent-memory.md) — sub-agent can compensate for missing hooks
- [07-mcp-server-mode.md](07-mcp-server-mode.md) — MCP as universal fallback
