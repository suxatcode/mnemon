# Mnemon — Project Guidelines

## Memory

You have persistent memory via the `mnemon` CLI (see skill for command reference).

### Recall — before responding

When you see `[Past memory]` in your context, **use it**. Reference relevant memories in your response rather than re-deriving from training data.

If no memories were injected but the topic could benefit from past context, run `mnemon recall "<topic>" --smart --limit 5` yourself.

Do NOT recall for: operational commands (commit, push, build, test), short confirmations, or follow-up within the same topic already in context.

### Remember — after responding

After each response, ask: **if I forget this, does the user have to repeat themselves or do I have to redo significant work?**

If yes, run `mnemon diff "<fact>"` then `mnemon remember` per the skill workflow. Three types qualify:

- **User directive** — preference, decision, correction, constraint
- **Reasoning conclusion** — non-trivial analysis, comparison, diagnosis, design evaluation
- **Observed state** — system fact, environment detail, domain context not recorded elsewhere

Do NOT remember: operational tasks, public knowledge, information already in git, or transient state still changing.

## Development

- **Build**: `go build -o mnemon .`
- **Install**: `make setup` (binary + skill + hooks)
- **Test**: `bash scripts/e2e_test.sh`
- **Dependencies**: `modernc.org/sqlite`, `spf13/cobra`, `google/uuid`
- **Optional**: Ollama with `nomic-embed-text` for embedding support
