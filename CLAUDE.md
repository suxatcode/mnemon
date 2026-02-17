# Mnemon — Project Guidelines

## What is this project
Mnemon is a standalone memory daemon for LLM agents, built in Go with SQLite storage and a MAGMA-inspired five-graph architecture (temporal, entity, causal, semantic, narrative edges).

## Persistent Memory (mnemon CLI)

You have access to a persistent memory system. **Use it actively** — it is the core product of this project.

### On conversation start
```bash
mnemon recall "<current topic>" --smart --limit 5
```
Always load relevant context before starting work.

### When you learn something worth remembering
```bash
# 1. Check for duplicates first
mnemon diff "<new fact>"
# 2. Based on suggestion:
#    ADD      → mnemon remember "<fact>" --cat <category> --imp <1-5>
#    CONFLICT → mnemon forget <old_id> && mnemon remember "<updated>" --cat <cat> --imp <n>
#    DUPLICATE→ skip
```

### When the user asks about past context
```bash
mnemon recall "<query>" --smart --limit 10
```

### What to remember
- **User preferences**: tool choices, coding style, workflow preferences → `--cat preference --imp 4`
- **Architectural decisions**: why we chose X over Y → `--cat decision --imp 5`
- **Key facts**: project structure, API specs, benchmarks → `--cat fact --imp 3`
- **Lessons learned**: debugging insights, patterns → `--cat insight --imp 4`
- **Project state**: current phase, blockers, WIP → `--cat context --imp 3`

### Rules
- Always `diff` before `remember` to avoid duplicates
- Use `--smart` on recall for intent-aware retrieval
- Do NOT store secrets, passwords, API keys, or tokens
- Prefer specific categories over `general`

### Semantic linking
After `mnemon remember`, check `semantic_candidates` in the output. For truly related candidates:
```bash
mnemon link <source_id> <target_id> --type semantic --weight 0.85
```

### Causal & entity enrichment
After `mnemon remember`, check `causal_candidates` and `entity_hints` in the output:
```bash
mnemon link <src> <tgt> --type causal --weight 0.8 --meta '{"sub_type":"causes"}'
mnemon enrich <id> --entities "X,Y" --rebuild-edges  # supplement entities
```

### Narrative consolidation
```bash
mnemon consolidate [--window 72h] [--min-cluster 3]  # find narrative clusters
mnemon consolidate --create --title "..." --members "id1,id2,id3"  # create narrative
```

### Retention review
Periodically review memory health:
```bash
mnemon gc --threshold 0.4       # list low-retention candidates
mnemon gc --keep <id>           # boost retention for a valuable insight
mnemon forget <id>              # purge stale insights
```

### Embedding (optional, requires Ollama)
When Ollama is running, `remember` auto-generates embeddings and `recall --smart` uses hybrid vector+keyword search.
```bash
mnemon embed --status          # check embedding coverage
mnemon embed --all             # backfill embeddings for all insights
mnemon embed <id>              # embed a specific insight
```
Install: `brew install ollama && ollama pull nomic-embed-text`

### Observability
```bash
mnemon log            # see recent operations (what was stored/queried)
mnemon status         # see memory statistics
```

## Development

- Go binary, dependencies: `modernc.org/sqlite`, `spf13/cobra`, `google/uuid`
- Optional: Ollama for embedding support (`nomic-embed-text`)
- Run tests: `./scripts/e2e_test.sh`
- Build: `go build -o mnemon .`
- Install: `go build -o $GOPATH/bin/mnemon .`
