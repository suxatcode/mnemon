<!-- mnemon:start -->
## Memory

You have persistent memory via the `mnemon` CLI.

### Recall — before responding

When you see `[Past memory]` in your context, **use it**. Reference relevant memories rather than re-deriving.

If no memories were injected but the topic could benefit from past context, run `mnemon recall "<topic>" --limit 5` yourself.

Do NOT recall for: operational commands, short confirmations, or follow-up within the same topic already in context.

### Remember — after responding

Ask: **if I forget this, will the user have to repeat themselves or will I redo significant work?**

Three types qualify: **user directive** (preference, decision, correction), **reasoning conclusion** (non-trivial analysis, design evaluation), **observed state** (system fact, environment detail).

If yes, store it: `mnemon remember "<fact>" --cat <cat> --imp <1-5> --entities "e1,e2" --source agent`

Do NOT remember operational/public/git-tracked/transient info.
<!-- mnemon:end -->
