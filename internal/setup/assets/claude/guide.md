### Recall — before responding

**Default: recall on every new user message**, unless ALL of these apply:
- Direct follow-up within a topic already fully in context
- No reference to past sessions, decisions, or preferences
- No knowledge dependency beyond the current conversation

**Before web search**: always recall first — stored context sharpens queries.

To recall: `mnemon recall "<query>" --limit 5`.
Craft a focused, keyword-rich query — do not pass the raw user prompt.

### Remember — after responding

Run this decision tree after every substantive response:

**Step 1 — Does this exchange contain any of these?**
  a) User directive — preference, decision, correction, explicit "remember this"
  b) Reasoning conclusion — non-trivial judgment from multi-source synthesis
  c) Durable observed state — system fact, environment detail, architectural finding
  → No to all → STOP.

**Step 2 — Does a highly overlapping memory already exist?**
  → Yes, incremental new info → UPDATE (merge into existing)
  → Yes, but contradicts/supersedes → REPLACE
  → No significant overlap → CREATE

**Step 3 — Is it worth storing?**
  Rebuilding from scratch costs more than storing + recalling?
  - Single-query public facts → No
  - Multi-source synthesis with non-obvious conclusions → Yes
  - User-specific context no search engine can recover → Yes
  → No → STOP.

**What to store**: conclusions and user-specific context, not raw facts.
**How to store**: delegate to a Task sub-agent (`subagent_type="Bash"`, `model="sonnet"`).
Only provide what to store — content, category, importance, entities, and create/update intent.
The sub-agent will read the mnemon skill and execute the correct commands itself.

Do NOT: write CLI commands or workflow steps in the sub-agent prompt (the sub-agent has access to the skill docs and will use the correct flags).
Do NOT run memory writes in the main conversation, or remember operational/public/git-tracked/transient info.
