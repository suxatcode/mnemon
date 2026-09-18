---
name: mnemon
description: Persistent memory CLI for Hermes Agent. Store facts, recall past knowledge, link related memories, manage lifecycle.
---

# mnemon

Use `mnemon` when durable memory can materially improve continuity across
Hermes sessions. Hooks may inject recalled context before an LLM call, but the
agent decides what is worth storing.

## Workflow

1. Recall when prior decisions, preferences, or facts may affect the current task:
   `mnemon recall "<query>" --limit 10`
2. Remember only stable, reusable knowledge:
   `mnemon remember "<fact>" --cat <cat> --imp <1-5> --entities "e1,e2" --source agent`
3. Link related memories after reviewing candidates from `remember`:
   `mnemon link <id> <candidate> --type <causal|semantic> --weight <0-1>`

## Commands

```bash
mnemon remember "<fact>" --cat <cat> --imp <1-5> --entities "e1,e2" --source agent
mnemon link <id1> <id2> --type <type> --weight <0-1> [--meta '<json>']
mnemon recall "<query>" --limit 10
mnemon search "<query>" --limit 10
mnemon import --dry-run <file>
mnemon import <file>
mnemon forget <id>
mnemon related <id> --edge causal
mnemon gc --threshold 0.4
mnemon gc --keep <id>
mnemon status
mnemon log
mnemon store list
mnemon store create <name>
mnemon store set <name>
mnemon store remove <name>
```

## Team memory

When a remote is configured (`mnemon auth login`), this is a **team-shared brain**, not a private notebook.

- Recall/search/related/status are **fully team-visible**. Personal layer is write-isolated, not read-isolated: you will see other people's personal notes. They are not private. Attribute every hit by `owner_principal` and `layer` in the JSON (`remember` also returns these fields).
- `layer: org` is organizational ground truth. Do not treat a coworker's personal notes as org policy.
- You may add, edit, `forget`, or GC **only your own** personal memories. Do not "update" or delete someone else's insight; the server rejects it.
- `mnemon link` is **not owner-scoped**. You may link any two insights the team can recall, including someone else's. Forget / GC / `--keep` stay owner-scoped.
- Org memories are written only by an organization principal (`role=org`). Regular skills never write org.
- Use `mnemon --local ...` only when you deliberately want the machine-local SQLite store instead of the team gateway.

## Guardrails

- Do not store secrets, passwords, tokens, private keys, or short-lived operational noise.
- Prefer concise insights over transcript dumps.
- Categories: `preference` · `decision` · `insight` · `fact` · `context`
- Edge types: `temporal` · `semantic` · `causal` · `entity`
- Max 8,000 chars per insight.
