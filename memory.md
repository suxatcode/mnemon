# Memory Skill — mnemon

You have access to a persistent memory system via the `mnemon` CLI.
Use it to store and retrieve facts, preferences, decisions, and context across sessions.

## Workflow

### On every conversation start
```bash
mnemon recall "<topic or project name>" --smart --limit 5
```
Load relevant context before responding.

### When you learn something worth remembering
```bash
# Check for duplicates first
mnemon diff "<new fact>"
# If suggestion is ADD:
mnemon remember "<fact>" --cat <category> --imp <1-5>
# If suggestion is CONFLICT: forget the old one, then remember the new one
mnemon forget <old_id>
mnemon remember "<updated fact>" --cat <category> --imp <1-5>
# If suggestion is DUPLICATE: skip
```

### When the user asks about past context
```bash
mnemon recall "<query>" --smart --limit 10
```

### Categories
- `preference` — user likes/dislikes, tool choices
- `decision` — architectural or design decisions with rationale
- `fact` — objective information, benchmarks, specs
- `insight` — patterns, lessons learned
- `context` — project state, environment info
- `general` — anything else

### Importance scale
- `5` critical — core architectural decisions, strong user preferences
- `4` high — important facts, recurring patterns
- `3` medium — general context (default)
- `2` low — minor details
- `1` trivial — ephemeral notes

### Other commands
```bash
mnemon search "<query>" --limit 10    # token-scored search
mnemon related <id> --edge causal     # find causally related insights
mnemon related <id> --edge entity     # find entity-linked insights
mnemon status                         # memory statistics
```

## Rules
- Always `diff` before `remember` to avoid duplicates
- Use `--smart` on recall for intent-aware retrieval
- Prefer specific categories over `general`
- Set importance >= 4 for decisions and strong preferences
- Do NOT store secrets, passwords, or tokens
