# MCP Server Mode

## Core Insight

MCP is a transport protocol, not an architecture. LLM-Supervised and MCP are orthogonal — Mnemon can expose tools via MCP while preserving LLM decision-making power.

## Problem

Most LLM CLIs support MCP. A single MCP server implementation would cover all CLIs without per-CLI adapters. But naive MCP implementation risks losing the LLM-Supervised advantage.

## Two MCP Designs

### Design A: Auto Mode (memcp-style)

```
tool: remember(content, category, importance)
  → store insight
  → auto-create all edges (temporal, entity, semantic, causal)
  → return: {id}
```

LLM has zero graph decision-making. Fast (1 round-trip) but loses supervision.

### Design B: Supervised Mode (Mnemon approach)

```
tool: diff(content)
  → return: {action: ADD|CONFLICT|DUPLICATE, existing_matches}

tool: remember(content, category, importance, entities)
  → store insight, DO NOT auto-create edges
  → return: {id, causal_candidates, semantic_candidates, entity_hints}

tool: link(source_id, target_id, type, weight, meta)
  → LLM decides which edges to create after reviewing candidates

tool: recall(query, smart, limit)
  → intent-aware retrieval

tool: forget(id)
  → soft delete

tool: gc(threshold) / gc_keep(id)
  → lifecycle management
```

LLM sees candidates, approves/rejects edges. 2-4 round-trips but full supervision preserved.

### Design C: Hybrid Mode (recommended)

```
tool: remember(content, category, importance, entities, auto_link=true|false)
  auto_link=true  → auto-create all edges (fast, for low-value facts)
  auto_link=false → return candidates for LLM review (precise, for decisions/insights)
```

Best of both worlds. The LLM chooses the level of involvement per insight.

## MCP vs CLI: What Changes

| Aspect | CLI Mode (current) | MCP Mode |
|--------|-------------------|----------|
| Transport | `bash mnemon remember ...` | MCP tool call |
| Output parsing | stdout text | structured JSON |
| Hook integration | Shell script | Depends on CLI |
| LLM decision model | Identical | Identical |
| Portability | Any CLI with bash | Any CLI with MCP |

The supervision model is identical. Only the transport changes.

## Architecture

```
mnemon mcp serve [--port PORT | --stdio]
  ├── stdio mode (default): MCP over stdin/stdout
  └── http mode: MCP over HTTP (for remote/multi-client)

Internally:
  MCP request → same Go engine (store, graph, search) → MCP response
```

No new dependencies. The MCP server is a thin wrapper over the existing engine.

## Auto-Recall Without Hooks

For CLIs without hooks, MCP can partially compensate:

```
Option 1: Rules file instructs LLM to call recall() at conversation start
Option 2: MCP server exposes a "session_start" tool that returns recent context
Option 3: CLI-specific adapter triggers recall via available mechanism
```

None is as reliable as a proper hook, but covers the long tail of CLIs.

## Implementation Sketch

```go
// cmd/mcp.go
var mcpCmd = &cobra.Command{
    Use:   "mcp",
    Short: "Start MCP server",
    RunE: func(cmd *cobra.Command, args []string) error {
        engine := graph.NewEngine(dataDir)
        server := mcp.NewServer(engine)
        return server.ServeStdio() // or ServeHTTP(port)
    },
}
```

MCP protocol handling: use a lightweight Go MCP library or implement the JSON-RPC subset directly (tools/list, tools/call).

## Priority vs Adapters

These two approaches are complementary, not competing:

```
Short-term: CLI adapters for top CLIs (Gemini, Codex, OpenCode)
  → Best experience where hooks exist

Medium-term: MCP server as universal fallback
  → Covers all MCP-capable CLIs with one implementation

Long-term: Both coexist
  → Hook-capable CLIs use adapters (auto-recall + MCP tools)
  → Hookless CLIs use MCP only (manual recall)
```

## Related

- [06-multi-cli-integration.md](06-multi-cli-integration.md) — adapter approach
- [01-sub-agent-memory.md](01-sub-agent-memory.md) — sub-agent for edge evaluation
