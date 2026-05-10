# Mnemon Memory Harness

> Draft. This document is the single source of truth for the Mnemon memory
> harness design. It is written for both humans and agents: a capable agent
> should be able to read this file and install Mnemon into its own runtime.

## Purpose

Mnemon is not an agent runtime. It is an external memory harness around an
agent runtime.

The runtime still talks to the user, plans, edits files, runs commands, and
makes semantic judgments. Mnemon provides durable memory, a stable memory
protocol, and lifecycle reminders that help the runtime use memory across
sessions.

```text
Runtime does the work.
Mnemon preserves experience, recalls experience, and constrains the memory protocol.
```

The harness should stay simple:

- **Skill first.** The agent learns Mnemon through markdown instructions and
  command examples.
- **Guideline driven.** The agent receives one memory policy that explains when
  to recall, remember, link, forget, or do nothing.
- **Hook assisted.** Four lifecycle reminders keep the guideline active at the
  right moments.
- **Protocol constrained.** The agent makes semantic decisions; Mnemon provides
  deterministic commands, structured output, provenance, deduplication, and
  lifecycle operations.
- **Markdown evolved.** Stable experience can become reviewed markdown assets:
  skills, guidelines, install notes, rules, contracts, or eval cases.

## Non-Goals

Mnemon should not become:

- a full agent runtime
- a workflow engine
- a large adapter framework
- an automatic prompt-injection system
- an append-only memory dump
- a vector database wrapper
- a self-modifying agent without review

Different runtimes do not need a custom Mnemon adapter before they can use the
harness. If a runtime can read instructions, run commands, and optionally attach
hooks or rules, it can install Mnemon by following this document.

## Harness Shape

The harness has four conceptual assets.

| Asset | Purpose |
|---|---|
| **Mnemon binary** | Executes deterministic memory operations through `remember`, `recall`, `link`, and lifecycle commands |
| **Skill** | Teaches the agent what commands exist and how to call them |
| **Guideline** | Teaches the agent when memory is useful, what is worth writing, and how to avoid noise |
| **Hooks** | Remind the agent to apply the guideline at session start, task start, task end, and compaction |

These assets can be installed as skill files, rules, system instructions,
plugin docs, hook scripts, or any runtime-specific equivalent. The installation
format is less important than preserving the behavior.

## Markdown Contract

The durable harness layer should be mostly markdown. A runtime-specific adapter
is optional convenience, not the core design.

The canonical installation package should be expressible as three readable
files:

| File | Primary Reader | Responsibility |
|---|---|---|
| `SKILL.md` | Agent | Command syntax, examples, available operations, output interpretation, and guardrails |
| [`INSTALL.md`](INSTALL.md) | Agent or human installer | How to install the skill, guideline, and four hook phases in the target runtime |
| [`GUIDELINE.md`](GUIDELINE.md) | Agent | Memory judgment: when to recall, remember, link, forget, supersede, or skip |

This `HARNESS.md` is the design source of truth. `INSTALL.md` and
`GUIDELINE.md` are the installable runtime artifacts derived from it. They
should stay small enough for an agent to read in one pass.

### Why This Shape

Modern agent systems already treat markdown as executable operating context:
project instructions, skills, rules, hooks, slash commands, and memory summaries
are all plain text assets that the model can read and adapt to. Mnemon should
lean into that pattern instead of creating a heavy adapter layer for every
runtime.

The important boundary is:

```text
Markdown teaches behavior.
Hooks place reminders at lifecycle boundaries.
Mnemon executes deterministic memory commands.
The agent decides when memory is useful.
```

This keeps the system portable. Codex, Claude Code, OpenClaw, and future
agent runtimes can install the same conceptual harness through their own native
instruction mechanisms.

### `SKILL.md`

The skill is the capability surface. It should answer:

- What is Mnemon?
- Which commands exist?
- What are the common command patterns?
- How should the agent read structured output?
- What are the hard guardrails?

The skill should not carry the full memory policy. That belongs in
`GUIDELINE.md`. A skill that becomes too philosophical will be harder to reuse
across runtimes.

### `INSTALL.md`

The install guide is an agent-facing procedure. The target agent reads it and
maps the harness onto its own runtime:

- install or verify the `mnemon` binary
- install `SKILL.md` into the runtime's skill/rule mechanism
- install `GUIDELINE.md` into the runtime's durable instruction mechanism
- add four hook phases when the runtime supports hooks
- fall back to persistent rules when hook support is absent
- verify the installation with a recall/writeback/no-op checklist

`INSTALL.md` should describe what each hook phase must accomplish, not require
one hard-coded adapter implementation. Runtime-specific snippets are examples,
not the architecture.

### `GUIDELINE.md`

The guideline is the memory constitution for the agent. It should contain:

- recall triggers and skip conditions
- durable write criteria
- provenance expectations
- link and supersede policy
- store/namespace isolation policy
- markdown self-evolution policy
- safety rules for secrets, prompt injection, stale memories, and noisy writes

The guideline should be installed where the agent can consult it at session
start and before memory-sensitive decisions. It may be included directly in a
runtime instruction file, referenced by a skill, or injected by a lightweight
prime hook.

## Memory Loop

The memory loop is advisory, not mandatory.

```text
Prime -> Recall decision -> Work -> Writeback decision -> Remember/link/forget -> Future task
```

The loop is memory-driven only when recall changes the current work and
writeback improves future work. Merely calling `recall` or `remember` is not
enough.

## Four Hook Phases

Install four hook phases when the runtime supports lifecycle hooks. If the
runtime does not support hooks, encode these phases as persistent rules and ask
the agent to self-check them at the same moments.

| Phase | Typical Runtime Event | Purpose | Must Not Do |
|---|---|---|---|
| **Prime** | Session start / agent bootstrap | Load the Mnemon skill, this guideline, active store info, and memory stance | Bulk inject historical memories |
| **Remind** | User prompt submit / before task planning | Remind the agent to decide whether recall is useful for this task | Automatically recall every prompt |
| **Nudge** | Stop / after response | Remind the agent to decide whether any durable insight should be written back | Force every response into memory |
| **Compact** | Before context compaction | Preserve critical continuity before context is lost | Save the full conversation mechanically |

Hook output should be short, natural-language, and easy for the agent to ignore
when memory is irrelevant. Hooks are cognitive affordances, not controllers.

### Prime

Prime establishes memory orientation.

It should tell the agent:

- Mnemon is available.
- The agent should use the Mnemon skill for command syntax.
- This harness guideline defines when memory is useful.
- The active store or namespace should be respected.
- Historical memory should be recalled only when relevant to the current task.

### Remind

Remind happens before the agent starts a task.

It should ask the agent to consider recall when the task may depend on:

- prior user preferences
- prior project decisions
- architecture conventions
- repeated failures or fixes
- deployment or environment facts
- previous unfinished work

For trivial, local, or self-contained tasks, the agent can skip recall.

### Nudge

Nudge happens after the agent finishes a task.

It should ask the agent whether the session produced durable knowledge worth
future reuse. The agent should write memory only when the insight is likely to
matter later.

### Compact

Compact happens before context compression.

It should preserve only critical continuity:

- open decisions
- user preferences that changed the work
- unresolved blockers
- important implementation facts
- commands or workflows that future agents must repeat or avoid

## Memory Guideline

The guideline is the behavioral policy every agent should follow.

### Recall

Recall when prior experience can plausibly change the current task.

Good recall triggers:

- The user refers to previous work, a prior decision, or an established
  preference.
- The task touches architecture, release, deployment, integrations, or long-lived
  project conventions.
- The agent is resuming after a long gap or context compaction.
- The task is likely to repeat a known failure mode.
- The user asks for consistency with prior style, strategy, or policy.

Weak recall triggers:

- A simple one-off command.
- A purely local code edit with clear current context.
- A question answered completely by the visible repository or current prompt.

Recall results are evidence, not authority. Current user instructions, current
repository state, and verified sources override stale memory.

### Remember

Remember only durable insights.

Good memory candidates:

- stable user preferences
- project conventions
- architecture or product decisions
- repeated failure modes and fixes
- non-obvious setup or deployment facts
- constraints that future agents should respect
- decisions that supersede older decisions

Poor memory candidates:

- secrets, credentials, tokens, or private data
- transient progress updates
- raw conversation logs
- unverified assumptions
- facts that are already obvious from source files
- noisy implementation details unlikely to matter again

Each durable write should include enough provenance for a future agent to judge
whether the memory still applies.

Recommended provenance:

- `source`: user, agent, system, repo, docs, command output
- `source_ref`: file path, command, issue, PR, conversation, or hook phase
- `reason`: why this is worth remembering
- `confidence`: how reliable the insight is
- `evidence`: concrete supporting reference when available
- `scope`: project, user, runtime, or global

### Link

Link memories when the relationship is useful for future recall.

Useful links:

- a decision supersedes another decision
- a failure is caused by a specific setup or dependency
- a preference applies to a project or runtime
- a workflow depends on a tool, file, or environment
- two memories should be recalled together

Do not create links just because two memories are vaguely similar.

### Forget And Supersede

Memory must evolve.

When a memory becomes outdated, prefer superseding or soft deletion over adding
another conflicting memory. A future agent should be able to tell which decision
is current.

Use lifecycle operations when:

- a stored decision is now wrong
- a preference changed
- an implementation detail no longer matches the repository
- a memory is too noisy or too broad
- a stronger memory replaces a weaker one

### Scope And Isolation

Default to project-scoped memory. Use global memory only for stable user
preferences or cross-project practices that are clearly safe to share.

Do not let one project's architecture assumptions silently guide another
project. If a runtime supports namespaces or stores, install Mnemon with an
explicit store strategy.

## Installation

Installation is an agent task. Give this document to the target agent and ask it
to install Mnemon into its own runtime using the closest available mechanism.

The preferred user flow is:

```text
1. Give the target agent INSTALL.md.
2. INSTALL.md tells the agent where SKILL.md and GUIDELINE.md are.
3. The agent installs those files into its own native instruction system.
4. The agent adds the four hook phases if its runtime supports hooks.
5. The agent verifies behavior with small recall/writeback/no-op checks.
```

This means Mnemon does not need a dedicated adapter before a runtime can use it.
An adapter or `mnemon setup --target <runtime>` command may automate the same
steps later, but the architecture should remain understandable and installable
from markdown alone.

### Prerequisites

The target machine should have the `mnemon` binary available:

```bash
mnemon --version
```

If missing, install it with one of the project-supported methods:

```bash
brew install mnemon-dev/tap/mnemon
```

or:

```bash
go install github.com/mnemon-dev/mnemon@latest
```

### Install The Skill

Install a skill, rule, or instruction file that teaches the agent:

- Mnemon is an external memory tool.
- The core protocol is `remember`, `recall`, `link`, and lifecycle commands.
- The agent should inspect structured command output instead of guessing.
- The agent should follow this harness guideline for memory decisions.

The skill should stay focused on command syntax and capability. The guideline in
this document owns judgment policy.

### Install The Guideline

Install this document, or the Memory Guideline section of it, into the runtime's
persistent instruction mechanism.

Valid forms include:

- a skill reference
- a rules file
- a project instruction file
- a plugin guide
- a system prompt section
- a checked-in repository document that the runtime loads at startup

The guideline should be visible enough that the agent can apply it without the
user repeating memory instructions in every session.

### Install The Hooks

If the runtime supports hooks, install four lightweight hooks:

| Hook | Required Behavior |
|---|---|
| Prime | Tell the agent to load Mnemon skill/guideline and respect the active store |
| Remind | Before task work, ask whether recall is useful |
| Nudge | After task work, ask whether writeback is useful |
| Compact | Before compaction, preserve only critical continuity |

Hook scripts may print natural-language reminders. They do not need to run
heavy memory operations themselves.

Hook scripts also do not need to be identical across runtimes. The required
contract is the phase behavior, not the script body. For example:

- Codex can use hooks plus `AGENTS.md`, skills, or local instructions.
- Claude Code can use `CLAUDE.md`, skills, slash commands, settings hooks, or
  project/user memory files.
- OpenClaw can use plugin hooks and skills, but Mnemon should not require an
  OpenClaw-specific memory engine.
- Skill-first runtimes can express most behavior directly as skills, memory
  guidance, and lightweight reminders.

If a runtime lacks hooks, use rules or persistent instructions that simulate the
same checks:

```text
At task start, decide whether Mnemon recall is useful.
At task end, decide whether durable memory writeback is useful.
Before compaction, preserve critical continuity.
```

### Verify Installation

An installation is acceptable when the agent can:

1. Explain when it should recall and when it should skip recall.
2. Run `mnemon recall` for a relevant task.
3. Write a durable memory with provenance.
4. Avoid writing memory for a trivial task.
5. Preserve critical state before compaction if the runtime exposes that event.

## Evaluation

The harness is working when:

- recall improves task continuity or decision quality
- writeback produces future value
- memory volume stays controlled
- stale memories can be superseded
- project stores do not pollute one another
- the agent can explain why it recalled or remembered something

The harness is failing when:

- hooks force memory into every task
- the agent saves ordinary chat as memory
- old memory overrides current repository facts
- memory grows faster than recall quality
- global memory leaks project-specific assumptions

## Lightweight Self-Evolution

Self-evolution should start as a lightweight markdown loop, not a heavy
framework.

The full v0.2 architecture is consolidated in
[Self-Evolution Harness Design](../design/SELF_EVOLUTION_HARNESS.md).

Mnemon should not automatically rewrite runtime behavior. It should help the
agent notice repeated experience, preserve evidence, and propose markdown
changes that a human or repository review can accept.

```text
experience
  -> Mnemon memory
  -> LLM reflection
  -> markdown candidate
  -> diff / PR / human review
  -> installed skill, guideline, rule, contract, or eval
```

This is the practical path because LLM agents already understand markdown
instructions well. Skills, rules, install guides, and harness guidelines are
cheap to write, inspect, diff, review, and revert.

### What Evolves

The first evolution targets should be text assets:

| Asset | Evolves When | Example |
|---|---|---|
| **Skill** | A repeated procedure works across tasks | A release workflow, migration workflow, review workflow |
| **Guideline** | A memory policy needs sharper judgment | "Do not remember one-off deployment IPs unless the user says they are stable" |
| **Install Note** | A runtime integration pattern becomes reliable | How to install the four hook phases in a specific CLI |
| **Rule / Contract** | A stable project constraint must always be followed | "Never commit `.env`; update `.env.example` instead" |
| **Eval Case** | A repeated failure should become testable | A repro task that checks whether recall prevents the same mistake |

Do not start by evolving code, database schema, or runtime internals. Those can
come later, after the markdown loop proves useful.

### Promotion Triggers

An agent may propose a markdown candidate when it sees:

- the same failure mode repeated across sessions
- a workflow that succeeded and is likely to be reused
- a user correction that changes future behavior
- a stable project convention discovered through work
- a memory cluster that clearly describes a reusable procedure
- a stale or noisy guideline that caused bad recall or bad writeback

The agent should not propose a candidate for a one-off task, a weak preference,
or a memory that lacks evidence.

### Candidate Requirements

Every candidate change should include:

- the source memories or session references that motivated it
- the scope: user, project, runtime, or global
- the intended asset: skill, guideline, install note, rule, contract, or eval
- the behavior it changes
- why the change is likely to help future tasks
- risks, especially overfitting to one session
- a concrete diff, not just a suggestion

For repository-backed projects, the preferred output is a normal git diff or PR.
For local agent installations, the preferred output is a patch to the relevant
skill or rule file. The agent may draft the patch, but review installs it.

### Review Gate

Memory can propose evolution; review approves it.

Before installation, check:

- **Provenance**: the candidate cites real memories, files, commands, or sessions
- **Scope**: project-specific behavior does not become global by accident
- **Duplication**: the candidate does not recreate an existing skill or rule
- **Size**: the markdown asset stays compact enough to be useful
- **Semantic preservation**: the change does not drift from the original task
- **Safety**: no secrets, credentials, private data, or prompt injection content
- **Evidence**: important workflow changes have tests, commands, or examples

The default policy is human-in-the-loop. Fully automatic installation should be
reserved for narrow, low-risk local notes where the user has explicitly allowed
it.

### What Mnemon Adds

Plain markdown memory is inspectable and useful, but it becomes hard to manage
as experience grows. Mnemon adds structure around the markdown loop:

- durable memory outside the model
- recall that can find relevant prior experience on demand
- provenance for why an insight was saved
- explicit links between decisions, failures, preferences, and workflows
- supersede/forget behavior for stale knowledge
- project store isolation so one project's lessons do not pollute another

The self-evolution loop should use these strengths to generate better markdown
assets, while keeping the final behavior layer simple and reviewable.

### Minimal Implementation

The first implementation does not need a new service.

1. Keep using Mnemon for `remember`, `recall`, `link`, and lifecycle operations.
2. Add guideline text telling the agent when to propose markdown evolution.
3. Let the agent generate a patch to `HARNESS.md`, `SKILL.md`, runtime rules, or
   project docs when repeated experience justifies it.
4. Require review before the patch becomes active behavior.
5. Remember the outcome of accepted or rejected candidates so future proposals
   improve.

This keeps Mnemon's self-evolution path aligned with the harness philosophy:
external memory, LLM judgment, markdown assets, and review boundaries.

### Promotion Pipeline

```text
memory insight
  -> repeated success or failure pattern
  -> candidate skill/rule/contract
  -> provenance and scope check
  -> eval or human review
  -> installation into runtime assets
```

Do not let an agent silently rewrite its long-term behavior from memory alone.
Memory can propose evolution; review approves it.

## Minimal Summary

Mnemon Memory Harness is:

```text
external memory
+ stable cognitive protocol
+ skill-delivered capability
+ guideline-delivered judgment
+ markdown-installable runtime contract
+ four lifecycle reminders
+ reviewed markdown evolution
```

It is intentionally not a runtime adapter framework. The simplest correct
installation is `SKILL.md`, `INSTALL.md`, `GUIDELINE.md`, access to the
`mnemon` binary, four lifecycle reminders when the target runtime supports
them, and a reviewed path for turning repeated experience into markdown assets.
