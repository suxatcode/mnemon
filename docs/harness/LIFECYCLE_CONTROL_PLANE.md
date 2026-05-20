# Lifecycle Control Plane

Chinese version: [LIFECYCLE_CONTROL_PLANE.md](../zh/harness/LIFECYCLE_CONTROL_PLANE.md)

This document defines the lightweight control model behind Mnemon Harness. The
visual site version is available at [Lifecycle Control Plane](../site/lifecycle-control-plane/index.html).

Mnemon does not need a heavy distributed control system. It needs a consistent
model for making agent lifecycle capabilities durable, observable, portable, and
governable.

## Minimal Definition

Mnemon keeps `State`, declares `Intent`, observes `Reality`, and uses
`Reconcile` to pull Reality back toward Intent. The result is written back into
State.

```text
State -> Intent -> Reality -> Reconcile -> State
```

This is the stable kernel. Concrete files, skills, hooks, host adapters, evals,
and proposals enter the kernel through profiles.

## Core Model

| Concept | Meaning |
| --- | --- |
| State | Durable truth owned by Mnemon, such as memory, skills, reports, proposals, audit, and status under `.mnemon`. |
| Intent | The lifecycle shape Mnemon wants the system to present. |
| Reality | The current real state of the host, project, tools, evals, and runtime. |
| Reconcile | The alignment mechanism that compares Intent with Reality and writes outcomes back into State. |

Execution surfaces are not part of the core model. They belong to the execution
layer: they are how Mnemon reaches host reality.

## Entity Profiles

Entities are not the model itself. Each entity declares a profile inside the
model.

| Profile | Meaning | Examples |
| --- | --- | --- |
| Template | Reusable definition, not necessarily reconciled. | `Loop` |
| Controlled | Needs ongoing alignment of Intent and Reality. | `LoopBinding`, `EvalRun`, future `Goal` |
| Surface | Expresses or reaches host capability. | `HostCapability`, `Projection` |
| Evidence | Observed fact from Reality, not a declarative object. | `Observation`, runtime status |
| Governance | Review, risk, and audit boundary. | `Proposal`, `Review`, `Audit` |

Only controlled entities need the full `spec/status/reconcile` shape. Other
profiles participate in reconcile differently.

## Current Entities

| Entity | Profile | Role |
| --- | --- | --- |
| `Loop` | Template | Reusable lifecycle capability package such as memory, skill, or eval. |
| `Binding` | Controlled | Binds one `Loop` to one host; suitable as the first full controlled object sample. |
| `HostCapability` | Surface | Describes static or dynamic capabilities a host can expose. |
| `Projection` | Surface | Lets the HostAgent see Mnemon's Intent. |
| `Observation` | Evidence | Lets Mnemon see the HostAgent's Reality. |
| `Proposal` / `Review` / `Audit` | Governance | Stores proposals, decisions, and immutable records when Reconcile cannot safely complete automatically. |

## Execution Surfaces

Execution surfaces explain how Mnemon reaches the host without mixing that
mechanism into the core model.

### Projection

Projection is the static direction: render Intent into a host-readable view.

Examples:

- `.codex/skills`
- `.claude/hooks`
- host config
- generated docs
- manifests

Projection lets the HostAgent see Mnemon's Intent.

### Observation

Observation is the dynamic direction: turn Reality into status, evidence, or
proposal input.

Examples:

- Codex appserver
- session APIs
- eval endpoints
- tool status
- runtime errors

Observation lets Mnemon see HostAgent Reality.

## What Memory-loop Proved

Mnemon's method is to take capabilities that are often built as heavy external
systems and reintroduce them into the host lifecycle through hooks, skills,
daemon work, canonical state, and reconcile.

`memory` validated this pattern for memory:

```text
external memory service
  -> hook + skill + .mnemon state
  -> prime / remind / nudge / compact lifecycle
  -> lifecycle-native memory capability
```

The lifecycle control plane generalizes the same pattern for self-improving
loops:

```text
standalone self-improvement loop
  -> hook + skill + daemon + HostCapability
  -> projection / observation / reconcile
  -> governable project evolution
```

## Relation To Autoresearch

Autoresearch is a useful reference because it demonstrates a constrained
self-improving loop:

```text
edit -> run -> evaluate -> keep/discard -> repeat
```

Mnemon does not clone an experiment platform. Mnemon borrows the discipline of
self-improving loops and makes them lifecycle-native, host-portable, and
governable.

In Mnemon, the decision space expands beyond keep or discard:

- repair
- validate
- propose
- review
- audit
- no-op

## Evolution Levels

Mnemon should grow through lightweight capability levels:

| Level | Shape |
| --- | --- |
| Profiles | Every entity declares a profile before becoming a full resource object. |
| Projection | Project Intent into the HostAgent. |
| Observation | Observe Reality through appserver, eval, tool status, and runtime evidence. |
| Governance | Let AI produce patches, reports, and proposals while review gates control risk. |

The goal is not to copy a large control system. The goal is a small, consistent
lifecycle model that can scale from memory to self-evolving agentic
projects.
