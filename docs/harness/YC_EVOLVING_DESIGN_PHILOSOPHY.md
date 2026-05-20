# YC Evolving Design Philosophy

Chinese version: [YC_EVOLVING_DESIGN_PHILOSOPHY.md](../zh/harness/YC_EVOLVING_DESIGN_PHILOSOPHY.md)

This note captures a design philosophy inspired by the YC Root Access talk
"How to Build a Self-Improving Company with AI" and the Chinese article
"YC合伙人：如何打造一家自我进化的AI原生公司". It is not an article archive.
It records the parts that should guide Mnemon's harness and lifecycle control
plane design.

## Core Thesis

An AI-native organization should not be understood as a traditional hierarchy
with AI tools attached to each employee. It can be understood as a set of
recursive, self-improving loops:

```text
signals -> policy -> tools -> quality gates -> learning
    ^                                             |
    |---------------------------------------------|
```

For Mnemon, this strengthens the core harness thesis:

Mnemon should not become an agent runtime, a workflow engine, or a memory store
alone. Mnemon should provide the lifecycle control layer that lets host agents
turn durable context, skills, policy, feedback, and execution results into
governed self-improvement loops.

## From Copilot To Self-Improving System

The article draws a useful distinction:

| Mode | Shape | Limit |
| --- | --- | --- |
| Copilot | AI helps a human perform an existing task faster. | The organization still depends on human coordination and manual improvement. |
| Self-improving loop | AI observes outcomes, identifies failures, proposes or applies fixes, and feeds results back into the system. | Requires readable context, deterministic tools, quality gates, and durable feedback. |

Mnemon should be designed for the second mode. A host agent may execute the
work, but Mnemon should help the surrounding system remember what happened,
detect drift, improve skills, update lifecycle state, and preserve reviewable
evidence.

## Company Brain And Canonical Context

The article's "company brain" maps directly to Mnemon's canonical state idea.
The valuable asset is not a transient dashboard, generated script, chat thread,
or host-specific plugin file. The valuable asset is readable, durable,
structured context:

- goals, decisions, policies, and constraints
- memory and summarized operating knowledge
- skills and their usage evidence
- reports, proposals, audit records, and review status
- host bindings and capability manifests
- validation outcomes and observed drift

In Mnemon terms, this state should live under `.mnemon` or another canonical
state root. Host-specific directories such as `.codex`, `.claude`, or future
plugin surfaces should be treated as projections that can be regenerated.

```text
canonical context
  durable memory, skills, policy, reports, proposals, audit
        |
        v
lifecycle control
  reconcile, validate, project, learn
        |
        v
host surfaces
  skills, hooks, app servers, tools, generated files
```

## Disposable Software, Durable Context

The article argues that generated internal software can become temporary while
business context and skills become the durable asset. This is a strong fit for
Mnemon's host projection model.

Mnemon should treat host-native assets as useful but replaceable:

- generated dashboards
- host skill files
- hook glue
- app-server configuration
- eval runners
- temporary workflow code

The durable layer is the lifecycle state that explains what these assets are
for, when they are stale, how they were validated, and whether they should be
regenerated.

## Loop Structure

The article's loop structure can be translated into Mnemon's lifecycle model:

```text
State
  durable context, skill lifecycle state, reports, proposals, status
        |
        v
Intent
  goals, policies, desired visibility, review boundaries
        |
        v
Projection
  host-readable skills, hooks, app servers, tools, eval surfaces
        |
        v
Reality
  user intent, repo diffs, host behavior, eval results, customer feedback
        |
        v
Reconcile
  compare Intent with Reality, then record action, no-op, or proposal
        |
        v
Updated State
```

This is the minimum trunk Mnemon should keep clear:

```text
State -> Intent -> Projection -> Reality -> Reconcile -> State
```

## Host Capability Surfaces

The article emphasizes deterministic tools, generated software, and quality
gates. In Mnemon, these should be represented as host capability surfaces rather
than Mnemon-owned execution runtimes.

Examples:

- Codex skills and project files
- Claude Code skills, hooks, and subagents
- Codex app-server endpoints
- eval runners and test commands
- repository files and generated dashboards
- databases, search indexes, and external APIs exposed through host tools

The host owns execution. Mnemon owns lifecycle coordination around that
execution: what should exist, how it is projected, how it is validated, what
failed, and what should change next.

## Quality Gates And Human Boundaries

The article does not imply full autonomy everywhere. It explicitly leaves
humans at the edge of the system for high-risk, novel, ethical, or emotionally
complex situations.

Mnemon should make this boundary explicit:

- low-risk observation and reporting can be automated
- projection validation can be automated
- skill and memory proposals can be generated automatically
- destructive changes require explicit review
- high-risk policy, security, data, or production changes require human gates
- audit records should preserve what happened and why

This keeps self-improvement reviewable instead of invisible.

## Design Implications For Mnemon

This philosophy supports several concrete Mnemon design choices:

1. Keep `.mnemon` as canonical lifecycle state.
2. Treat `.codex`, `.claude`, and similar directories as projections.
3. Model each improvement path as a loop with signals, policy, tools, gates,
   and feedback.
4. Keep host execution outside Mnemon core.
5. Make Reconcile explicit: compare desired lifecycle state with actual host
   surfaces and observed outcomes.
6. Record status, failures, stale projections, and missing capabilities as
   first-class state.
7. Prefer generated or projected host assets over hand-maintained duplicated
   truth.
8. Preserve human review boundaries for risky changes.

## Strategic Position

This article describes the organizational shape Mnemon should serve:
self-improving agentic systems that operate through durable context and
recursive loops.

Mnemon's differentiation is not "memory for agents" by itself. The stronger
position is:

```text
Mnemon turns durable context into lifecycle-controlled agent improvement loops.
```

Memory is the continuity point. The loop is the differentiator. The control
plane is the product shape.
