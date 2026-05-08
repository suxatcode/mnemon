# 09. Anti-Patterns

The harness is valuable only if it remains installable into existing agents. These anti-patterns are architectural red lines: each one turns a harness into an agent framework or makes self-evolution unreviewable.

## Red-Line Test

Before adding a feature, ask:

```text
Can a generic agent still install the harness by reading INSTALL.md and GUIDELINE.md?
Can the feature degrade to proposal-only Markdown artifacts?
Can the host remain the owner of LLM loop, prompt assembly, tool routing, hooks, scheduler, UI, and permissions?
```

If any answer is no, the feature is probably outside the harness core.

## Anti-Pattern A: Prompt Assembler In Harness

Bad:

- harness builds the full system prompt;
- harness decides final instruction priority;
- harness injects memory into live turns without host mediation.

Correct:

- harness provides guideline, recall output, and prompt templates;
- host decides how to assemble the live prompt;
- recall output is short, bounded, and inspectable.

## Anti-Pattern B: Tool Router In Harness

Bad:

- runner decides which tools the agent may call;
- harness intercepts shell/file/network tool calls;
- skill execution bypasses host permissions.

Correct:

- host owns tool routing and permission model;
- harness provides write allowlists, validation scripts, and reports;
- jobs can call only declared host commands or thin deterministic scripts.

## Anti-Pattern C: Hidden LLM Client

Bad:

- runner embeds its own model SDK and key;
- maintenance jobs call arbitrary models outside host policy;
- background review uses tools that foreground agent would not have.

Correct:

- LLM jobs call a declared host command;
- missing host command downgrades to manual/proposal-only;
- output schema validation happens before any apply.

## Anti-Pattern D: File Watcher That Mutates Opportunistically

Bad:

- daemon watches the whole repo and rewrites memory/skills as files change;
- mutation timing is unrelated to host lifecycle events;
- user cannot trace why a change happened.

Correct:

- writes happen through semantic events, queued jobs, manual commands, or scheduled ticks;
- every mutation has report and ledger records;
- foreground activity can defer maintenance.

## Anti-Pattern E: Memory Database Replaces Markdown Control Plane

Bad:

- all memory moves into an opaque vector/database layer;
- hot behavior cannot be reviewed as text;
- retrieval output becomes the only source of truth.

Correct:

- Markdown remains the behavior control plane;
- cold memory can use indexes/databases as implementation detail;
- hot/warm/cold promotion is explicit and report-backed.

## Anti-Pattern F: Unlimited Skill Creation

Bad:

- every successful workaround becomes a new skill;
- skills duplicate each other;
- session details become permanent behavior.

Correct:

- patch existing skills first;
- create umbrella skills for class-level patterns;
- curator consolidates self-authored skills;
- one-off details remain session summaries or cold evidence.

## Anti-Pattern G: Auto-Mutating User Or Package Assets

Bad:

- curator rewrites user-authored guidance;
- package skills are silently edited in place;
- imported community skills are treated as disposable.

Correct:

- provenance controls curation eligibility;
- user/package/imported/pinned artifacts default to protected;
- package changes are proposed as forks, overlays, or upgrade reports.

## Anti-Pattern H: Policy Changes Through Self-Evolution

Bad:

- reflection changes safety policy;
- dreaming rewrites install behavior;
- eval constraints are updated to make a proposal pass.

Correct:

- `GUIDELINE.md`, `INSTALL.md`, hooks, schemas, and eval policy require human approval;
- high-risk changes become PR-style reports;
- evaluator constraints are protected.

## Anti-Pattern I: Hot Memory As Transcript Cache

Bad:

- hot memory accumulates raw history;
- long facts are appended until context budgets fail;
- old notes are silently dropped when size grows.

Correct:

- hot memory is short and declarative;
- warm memory holds capsules and candidates;
- cold memory holds evidence/transcripts/indexes;
- budget pressure creates demotion proposals, not silent truncation.

## Anti-Pattern J: Maintenance Marketed As Intelligence

Bad:

- daemon is described as the "brain" of the system;
- runner has separate goals or autonomy;
- maintenance jobs compete with active user tasks.

Correct:

- runner is cron + lease + ledger;
- jobs are bounded and inspectable;
- foreground user task always has priority.

## Anti-Pattern K: Host-Native State As Source Of Truth

Bad:

- each host stores memory/skills in its own native files with no canonical index;
- installer treats `CLAUDE.md`, `AGENTS.md`, and native skill dirs as mutable primary state;
- curator scans random host templates and cannot tell generated content from user content.

Correct:

- `.mnemon` is canonical filesystem;
- host-native files contain pointers, managed blocks, or generated projections;
- host-owned content outside markers is never silently rewritten;
- projection drift writes a report before overwrite.

## Architecture Checklist

A proposed component belongs in the harness only if:

1. it can be expressed as Markdown, schema, thin script, hook template, report, or optional job descriptor;
2. it can run without owning the host agent loop;
3. it can be disabled without losing manual skill operation;
4. it has explicit input/output contracts;
5. it writes reports for durable changes;
6. it respects provenance and protected targets;
7. it can degrade to proposal-only.

Otherwise, it should be a host feature, host binding, or external implementation.
