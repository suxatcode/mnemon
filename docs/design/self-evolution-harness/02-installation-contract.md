# 02. Hook-Based Agent-Agnostic Installation

Installation is not an adapter and not a host-specific runtime. Installation means:

```text
host agent reads INSTALL.md
  -> understands the semantic hook contract
  -> maps host lifecycle events to recall / observe / reflect / curate
  -> exposes the core skills
  -> points host instructions at .mnemon
  -> records the binding
```

The first installation path should be agent-executed. Any capable agent can read `INSTALL.md`, inspect its own host environment, and bind the harness using the host's native instruction, skill, hook, and scheduler surfaces. Later scripts may automate the same steps, but scripts do not define a second authority.

## Core Principle

The harness defines semantic hooks. The host chooses how to implement them.

| Harness concept | Host-specific realization |
|---|---|
| `recall` | session start, user prompt submit, pre-model call, or manual skill |
| `observe` | pre-tool, post-tool, approval result, error handler, or session summary |
| `reflect` | post-answer, stop, session end, conversation close, or manual skill |
| `curate` | idle task, scheduled task, cron, manual skill, or optional runner tick |

The contract is semantic, not API-specific. A host with native hooks can install L2/L3 behavior. A host with only Markdown can still install L0/L1 by exposing the same operations as manual skills.

## What Gets Installed

The minimal installed surface is small:

```text
.mnemon/
  INSTALL.md
  GUIDELINE.md
  harness.yaml
  fs.yaml
  skills/core/
    install/SKILL.md
    recall/SKILL.md
    observe/SKILL.md
    reflect/SKILL.md
    curate/SKILL.md
  hooks/
    recall.md
    observe.md
    reflect.md
    curate.md
  memory/
  state/
  reports/
  bindings/
```

Host-native files should only receive pointers, managed blocks, hook bindings, or projected skill entries. Long memory, long guidelines, and durable state stay in `.mnemon`.

## Semantic Hook Contract

Every hook receives a bounded event envelope and returns either a bounded result, a report, or a proposal.

```yaml
hook_event:
  hook: recall|observe|reflect|curate
  event_id: string
  host: string
  cwd: string
  trigger: string
  timestamp: string
  payload: object
  budgets:
    latency_ms: 0
    output_chars: 0
  permissions:
    writable_targets: []
    protected_targets: []
```

Hook output:

```yaml
hook_result:
  hook: recall|observe|reflect|curate
  event_id: string
  status: ok|none|proposal|blocked|error
  prompt_addition: string
  writes:
    - target: string
      action: create|patch|append|report
      status: applied|proposed|blocked
  report: string
  warnings: []
```

Rules:

- `recall` may return `none`; irrelevant memory is a valid result.
- `observe` writes evidence, usage signals, or reports; it should not directly rewrite Prompt Memory.
- `reflect` may patch allowlisted low-risk targets or write proposals.
- `curate` defaults to dry-run/proposal unless the host explicitly provides safe write enforcement.
- If the host cannot enforce writable targets, all durable mutations degrade to proposal-only.
- Every durable mutation writes a report.

## Agent Installation Loop

The host agent installs the harness by following this loop:

```text
read .mnemon/INSTALL.md
  -> read .mnemon/harness.yaml
  -> inventory host surfaces
  -> choose capability level
  -> produce install plan
  -> ask user approval for host-owned edits
  -> write managed instruction pointer
  -> expose core skills
  -> bind semantic hooks when supported
  -> record .mnemon/bindings/active.json
  -> run smoke tests
  -> write reports/install/<timestamp>.md
```

Inventory should detect only capabilities, not product identity:

| Surface | Questions |
|---|---|
| Instruction surface | Where can the host read persistent project instructions? |
| Skill surface | Can the host discover `SKILL.md` directories or equivalent commands? |
| Hook surface | Can the host call something on session, model, tool, or stop events? |
| Scheduler surface | Can the host run idle/scheduled maintenance? |
| Permission surface | Can the host restrict write targets? |
| Report surface | Where can the host write human-readable reports? |

Host identity is useful for scripts, but the architecture should not require hardcoded host maps.

## Capability Levels

| Level | Required host capability | Installed behavior |
|---|---|---|
| L0 Manual | can read Markdown | user/agent manually reads `GUIDELINE.md` and core skills |
| L1 Instruction | persistent instruction surface | managed pointer tells the host where `.mnemon` lives |
| L2 Hooks | lifecycle or tool hooks | `recall`, `observe`, and `reflect` run from host events |
| L3 Maintenance | idle/scheduled hook or external scheduler | `curate` and dreaming jobs run outside foreground work |
| L4 Eval | CI or repeatable test surface | higher-risk proposals run checks before merge |

The installer chooses the highest safe level. It must never emulate missing host capabilities by becoming an agent runtime.

## `harness.yaml`

`harness.yaml` is a manifest for agents and future scripts:

```yaml
harness:
  name: self-evolution-harness
  version: 0.1.0
  schema_version: 1
  description: Agent-agnostic self-evolution harness installed through semantic hooks.

paths:
  root: .mnemon
  install: INSTALL.md
  guideline: GUIDELINE.md
  fs: fs.yaml
  skills: skills/core
  hooks: hooks
  memory: memory
  state: state
  reports: reports
  bindings: bindings

semantic_hooks:
  recall:
    skill: skills/core/recall/SKILL.md
    template: hooks/recall.md
    preferred_triggers: [session_start, user_prompt, pre_model_call]
    fallback: manual_skill
  observe:
    skill: skills/core/observe/SKILL.md
    template: hooks/observe.md
    preferred_triggers: [pre_tool_call, post_tool_call, approval_result]
    fallback: session_summary
  reflect:
    skill: skills/core/reflect/SKILL.md
    template: hooks/reflect.md
    preferred_triggers: [turn_delivered, stop, session_end]
    fallback: manual_skill
  curate:
    skill: skills/core/curate/SKILL.md
    template: hooks/curate.md
    preferred_triggers: [idle_tick, scheduled_tick, manual_review]
    fallback: manual_skill

write_policy:
  default_mode: proposal
  auto_apply_allowed:
    - reports/**
    - state/usage.json
  protected_targets:
    - INSTALL.md
    - GUIDELINE.md
    - harness.yaml
    - hooks/**
    - eval/**

upgrade:
  preserve:
    - memory/**
    - state/usage.json
    - reports/**
    - archive/**
  report_dir: reports/install
```

## `INSTALL.md`

`INSTALL.md` should tell any agent how to install the harness without knowing the host in advance:

```text
# INSTALL.md

Goal:
Install Mnemon as a harness, not as a replacement agent runtime.

Read:
- .mnemon/harness.yaml
- .mnemon/GUIDELINE.md
- .mnemon/fs.yaml
- .mnemon/skills/core/*/SKILL.md

Find host surfaces:
- persistent instruction file or system prompt extension
- native skill directory or command registry
- lifecycle/tool hooks
- scheduler/cron/idle jobs
- write permission and approval boundaries

Bind semantic hooks:
- recall -> before context is assembled or as manual skill
- observe -> around tool calls or as session summary
- reflect -> after answer delivery or session end
- curate -> idle/scheduled/manual maintenance

Write policy:
- ask before editing host-owned config
- write only managed markers or generated binding files
- keep durable memory/state/reports in .mnemon
- downgrade to proposal-only when write limits cannot be enforced

Verify:
- host can find .mnemon/GUIDELINE.md
- host can invoke recall and receive bounded context or NONE
- observe can write a report or evidence record
- reflect can write a proposal report
- curate can run dry-run
- reinstall is idempotent
```

## Managed Instruction Pointer

Any instruction surface should receive only a compact pointer:

```markdown
<!-- mnemon:start -->
Mnemon self-evolution harness is installed for this workspace.

Read `.mnemon/GUIDELINE.md` for behavior rules.
Use `.mnemon/skills/core/recall/SKILL.md` before context injection when relevant.
Use `.mnemon/skills/core/observe/SKILL.md` around tool/evidence events when available.
Use `.mnemon/skills/core/reflect/SKILL.md` after completed work.
Use `.mnemon/skills/core/curate/SKILL.md` for maintenance.

Do not copy long memory into this file. `.mnemon` is canonical.
<!-- mnemon:end -->
```

The host owns everything outside the marker.

## Binding Record

After installation, the agent writes the actual binding it chose:

```yaml
binding:
  schema_version: 1
  host_label: detected-by-agent
  capability_level: L2
  canonical_root: .mnemon
  instruction_surface:
    path: AGENTS.md
    mode: managed_pointer
    marker: mnemon
  skill_surface:
    mode: native|pointer|manual
    targets: []
  hooks:
    recall:
      trigger: user_prompt
      mode: host_hook
      target: .mnemon/hooks/recall.md
    observe:
      trigger: post_tool_call
      mode: host_hook
      target: .mnemon/hooks/observe.md
    reflect:
      trigger: session_end
      mode: host_hook
      target: .mnemon/hooks/reflect.md
    curate:
      trigger: manual
      mode: manual_skill
      target: .mnemon/skills/core/curate/SKILL.md
  write_policy:
    enforced_by_host: true
    default_mode: proposal
  installed_at: "2026-05-09T00:00:00Z"
```

This record is descriptive. The source of authority remains `.mnemon` plus the host's own hook configuration.

## Verification

Smoke tests:

1. The host instruction surface points to `.mnemon/GUIDELINE.md`.
2. `recall` returns bounded context or `none`.
3. `observe` can write a report under `.mnemon/reports/`.
4. `reflect` can classify a completed turn into memory, skill, evidence, or report-only.
5. `curate` can run dry-run without mutating protected targets.
6. Reinstall updates the managed marker in place.
7. Removing host bindings does not delete memory, reports, or state.

## Scripted Installer Later

A future script may automate detection and file edits, but it must implement the same agent-readable protocol:

- read `INSTALL.md` and `harness.yaml`;
- generate the same install plan;
- ask for the same approvals;
- write the same binding record;
- run the same smoke tests;
- preserve the same proposal-only fallback.

Scripts are convenience, not a required runtime dependency.

## Acceptance Criteria

Installation design is acceptable when:

1. an arbitrary capable agent can install by reading Markdown;
2. host-specific knowledge is optional optimization, not architectural dependency;
3. the four semantic hooks can be mapped to native hooks or manual skills;
4. `.mnemon` remains canonical;
5. host-owned content outside markers is never overwritten;
6. missing hook support degrades to manual/proposal mode;
7. every installation writes an audit report and binding record.
