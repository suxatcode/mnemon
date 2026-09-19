---
name: mnemon
description: Persistent memory CLI for LLM agents. Store facts, recall past knowledge, link related memories (including a teammate's), manage lifecycle.
---

# mnemon

## Workflow

1. **Remember**: `mnemon remember "<fact>" --cat <cat> --imp <1-5> --entities "e1,e2" --source agent`
   - Diff is built in: duplicates are skipped, conflicts are auto-replaced.
   - Output includes `action` (added/updated/skipped), `semantic_candidates`, and `causal_candidates`.
2. **Link** (evaluate candidates from step 1 using judgment):
   - On a team remote you MAY link another teammate's memory to yours (or two teammates' memories). That is how the shared graph is built. Do not forget or update their content.
   - Review `causal_candidates`: link only when the memories are genuinely causally related.
   - Review `semantic_candidates`: high `similarity` alone is not enough; skip unrelated keyword matches.
   - Syntax: `mnemon link <id> <candidate> --type <causal|semantic> --weight <0-1> [--meta '<json>']`
3. **Recall**: `mnemon recall "<query>" --limit 10`

## Commands

```bash
mnemon remember "<fact>" --cat <cat> --imp <1-5> --entities "e1,e2" --source agent
mnemon link <id1> <id2> --type <type> --weight <0-1> [--meta '<json>']  # any two recallable team memories, including a coworker's
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

## Import Historical Chats

When the user asks to import old chats, notes, or exported context, create a
`memory_draft.json` with `schema_version: "1"`, `insights` entries containing
`content`, `category`, `importance`, `tags`, `entities`, and optional
`created_at`, plus optional `edges` using `source_index`, `target_index`,
`edge_type`, `weight`, and `reason`. Run `mnemon import --dry-run <file>`,
then run `mnemon import <file>` only after validation passes. After import,
verify with `mnemon status` and a focused `mnemon search` or `mnemon recall`.
Check the output `errors` field because imports can partially succeed.

## Team memory

When a remote is configured (`mnemon auth login`), this is a **team-shared brain**, not a private notebook.

- Recall/search/related/status are **fully team-visible**. Personal layer is write-isolated, not read-isolated: you will see other people's personal notes. They are not private. Attribute every hit by `owner_principal` and `layer` in the JSON (`remember` also returns these fields).
- `layer: org` is organizational ground truth. Do not treat a coworker's personal notes as org policy.
- You may add, edit, `forget`, or GC **only your own** personal memories. Do not "update" or delete someone else's insight; the server rejects it.
- `mnemon link` is **not owner-scoped**. You MAY link another teammate's memory to yours, or two teammates' memories — that is the shared graph. Forget / GC / `--keep` stay owner-scoped.
- Org memories are written only by an organization principal (`role=org`). Regular skills never write org.
- Use `mnemon --local ...` only when you deliberately want the machine-local SQLite store instead of the team gateway.

## Guardrails

- Use memory only when it can materially improve continuity or task quality.
- Do not store secrets, passwords, tokens, private keys, or short-lived operational noise.
- Categories: `preference`, `decision`, `insight`, `fact`, `context`
- Edge types: `temporal`, `semantic`, `causal`, `entity`
- Max 8,000 chars per insight.
