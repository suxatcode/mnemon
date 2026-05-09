# 04. Skills 与 Hooks

Harness 的行为能力主要通过 skill 表达；自动触发通过 hook 表达。Host 不支持 hook 时，skill 仍可手动调用。完整的 skill 生产路径见 [08-skill-production-paths.md](08-skill-production-paths.md)。

## Skill Production And Governance Paths

Harness recognizes three skill production entrances and one governance path. They differ by trigger, provenance, and auto-curation eligibility. This section is the hook-level summary; the detailed architecture is in `08`.

| Path | Trigger | Output | Provenance | Auto-curation |
|---|---|---|---|---|
| User-declared production | User explicitly asks to save or update a procedure | protected patch/create skill or proposal | `user` / `foreground` | no by default |
| Agent-offered production | Agent asks after a difficult task; user confirms | protected patch/create skill or proposal | `agent` + `foreground` | no by default |
| Background review production | `turn_delivered` / `Stop` / `SessionEnd` reflection | self-authored patch/create/support file or report | `agent` + `background_review` | yes, if self-authored and not pinned |
| Curator governance | curator/dreaming runner or scheduled job | umbrella skill, consolidation, archive/demotion proposal | `agent` + `curator` | yes, within allowlist |

Rules:

- Foreground user-created and user-confirmed skills belong to the user and must not be silently curated.
- Post-turn review may create or patch skills only when host can enforce write targets; otherwise it writes proposal reports.
- Curator governs library shape across time; it is not a per-turn production entrance.
- Dreaming may surface repeated workflow signals, but writes still go through the same skill_manage path.
- Curator should prefer umbrella skills and support files over one-session skills.
- Every path writes usage/provenance metadata.
- High-risk skills, policy skills, hook mounting policy, and installed hooks require human approval.

## Core Skills

### `install`

Purpose: install or upgrade the harness by mapping semantic hooks into the current host.

Responsibilities:

- Detect host capabilities and surfaces.
- Read `harness.yaml`.
- Build install plan.
- Apply only approved changes.
- Write install report.

Never:

- Delete user memory.
- Reset usage sidecar.
- Modify host config without approval.

### `recall`

Purpose: retrieve short context for current task.

Inputs:

- user prompt or task summary.
- cwd/project identity.
- optional files/branch/session id.

Outputs:

- short recall context.
- `NONE` if not relevant.

Rules:

- Prefer Prompt Memory because it is already in the host prompt snapshot.
- Long-term recall must be summarized and evidence-linked.
- Never inject raw transcript.
- Keep output below host budget.

### `observe`

Purpose: collect evidence without making durable conclusions.

Inputs:

- tool call args/result.
- errors.
- user corrections.
- approval/denial signals.

Outputs:

- episodic evidence/event file.
- optional usage signal.
- no semantic long-term write by default.

### `reflect`

Purpose: post-turn self-improvement review.

Outputs:

- memory add/replace proposal.
- skill patch proposal.
- new class-level skill proposal.
- report.

Rules:

- facts/preferences -> memory.
- workflows/procedures -> skill.
- task progress -> session summary only.
- patch existing skill before creating new skill.
- if host cannot enforce allowlist, proposal-only.

### `curate`

Purpose: long-term maintenance.

Inputs:

- `state/usage.json`.
- active skills.
- Prompt Memory.
- long-term recall/index summaries.
- consolidation proposals.
- reports.

Outputs:

- consolidation proposals.
- demotion/promotion proposals.
- archive proposals.
- curator report.

Rules:

- default dry-run.
- archive over delete.
- skip pinned.
- skip package/harness/imported/user-created unless approved.

### `research`

Purpose: preserve external/source-level research evidence.

Outputs:

- source map.
- fact/evidence distinction.
- research report.

Rules:

- cite source URLs.
- mark inference separately.
- do not promote unverified claims to Prompt Memory.

## Hook Templates

All hooks use the same envelope:

```text
semantic event + idempotency key + payload + budget
  -> scoped skill/prompt/script
  -> status + bounded output + optional report/proposal
```

Required hook semantics:

- retries must be idempotent;
- every hook has latency and output budgets;
- `none` is a valid status for recall;
- mutation-capable hooks must declare write permission up front;
- timeout/failure degrades to no-op or proposal-only;
- hooks never override the active user request.

### Recall Hook

Semantic events:

- `session_start`
- `pre_llm_call`
- `user_prompt_submit`

Host action:

1. Gather current prompt, cwd, session id.
2. Run `skills/recall` or `prompts/recall.md`.
3. Inject short output into current turn.

Boundary:

- No persistent writes.
- No long history.
- No override of current user request.

### Observe Hook

Semantic events:

- `pre_tool_call`
- `post_tool_call`
- approval request/response
- file changed

Host action:

1. Redact secrets.
2. Save evidence under `memory/longterm/episodic/evidence/`.
3. Update usage if relevant.

Boundary:

- Evidence only.
- No conclusions in Prompt Memory.
- If output contains secrets, discard or redact.

### Reflect Hook

Semantic events:

- `turn_delivered`
- `stop`
- `session_end`
- `subagent_stop`

Host action:

1. Run reflection prompt over recent conversation summary.
2. Restrict write targets if host supports it.
3. If not restricted, write proposals only.
4. Write report.

Auto-apply conditions:

```text
risk == low
AND target in write allowlist
AND host can enforce target restriction
AND not protected
AND not pinned/package/imported
```

Otherwise, proposal-only.

### Delayed Reflection Fallback

When host cannot run post-turn hooks, it may write a bounded session summary to the runner queue:

```text
state/jobs/queue/reflect/<session-id>.json
```

The queued job is processed by manual `reflect`, host scheduler, external cron, or optional runner. This is weaker than immediate background review, but preserves the same contract:

- summary/evidence in;
- memory-or-skill classification;
- proposal report out;
- allowlisted low-risk patch only when enforcement exists.

### Curate Hook

Semantic events:

- `idle_tick`
- `scheduled_tick`
- `runner_tick`
- manual command

Host action:

1. Load usage sidecar.
2. Identify stale or overlapping artifacts.
3. Produce dry-run report.
4. On explicit apply, snapshot first.
5. Apply allowlisted archive/patch.

Boundary:

- Default dry-run.
- Never delete; archive only.
- Never mutate protected targets without approval.

## Prompt Templates

Prompt templates should be scoped, not generic agent prompts.

Reflection prompt must include:

```text
You are not continuing the user task.
You may only propose or apply durable memory/skill changes.
Do not save one-off task progress.
Facts/preferences go to Prompt Memory.
Procedures/workflows go to skills.
If write-target restrictions are unavailable, output proposals only.
```

Curator prompt must include:

```text
Build umbrella skills.
Do not create one-session-one-skill.
Skip pinned/package/imported/user-created artifacts unless explicitly approved.
Archive over delete.
Write structured report.
```

## Fallback Behavior

| Host capability | Behavior |
|---|---|
| No skill system | Use Markdown files and instruction snippets |
| No hooks | Manual `recall`/`reflect`/`curate` skills |
| No write allowlist | Reports only, no direct patch |
| No scheduler | Manual curator or external cron |
| No CI | Eval proposals only |

Fallbacks are first-class behavior, not degraded hacks. They keep the harness installable across agents.
