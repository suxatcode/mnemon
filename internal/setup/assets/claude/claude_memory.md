<!-- mnemon:start -->
## Memory

You have persistent memory via the `mnemon` CLI (see skill for command reference).

### Recall — before responding

When you see `[Past memory]` in your context, **use it**. Reference relevant memories rather than re-deriving.

If no memories were injected but the topic could benefit from past context, run `mnemon recall "<topic>" --limit 5` yourself.

Do NOT recall for: operational commands, short confirmations, or follow-up within the same topic already in context.

### Remember — after responding

Ask: **if I forget this, will the user have to repeat themselves or will I redo significant work?**

Three types qualify: **user directive** (preference, decision, correction), **reasoning conclusion** (non-trivial analysis, design evaluation), **observed state** (system fact, environment detail).

If yes, **delegate to a Task sub-agent** (`subagent_type="Bash"`, `model="sonnet"`). Only provide what to store — content, category, importance, entities. The sub-agent will read the mnemon skill and execute the correct commands itself.

Do NOT: write CLI commands or workflow steps in the sub-agent prompt (the sub-agent has access to the skill docs and will use the correct flags). Do NOT run memory writes in the main conversation, or remember operational/public/git-tracked/transient info.
<!-- mnemon:end -->
