# 02. 安装契约

## 安装流程

安装不是运行 adapter，而是生成 host-specific binding。

```text
read harness.yaml
  -> detect host
  -> sense existing host templates
  -> choose capability level
  -> create/update `.mnemon` canonical filesystem
  -> build install plan
  -> dry-run report
  -> user approval if needed
  -> merge instruction snippet / managed block
  -> register/copy/symlink skill projections
  -> install hook templates if host supports hooks
  -> write projection metadata
  -> initialize memory/state/report dirs
  -> write state/install.json
  -> verify
```

安装必须幂等。重复安装不能重复插入 instruction snippet，不能重置 memory/state，不能覆盖用户修改。

## `harness.yaml`

`harness.yaml` 是机器可读 manifest。建议最小结构：

```yaml
harness:
  name: self-evolution-harness
  version: 0.1.0
  schema_version: 1
  description: Agent-agnostic self-evolution harness installed through skills and hooks.

capabilities:
  required:
    - read_markdown
    - write_reports
  optional:
    - native_skills
    - lifecycle_hooks
    - scheduled_tasks
    - maintenance_runner
    - eval_ci

paths:
  root: .mnemon/
  guideline: GUIDELINE.md
  install: INSTALL.md
  fs: fs.yaml
  skills: skills/
  hooks: hooks/
  prompts: prompts/
  schemas: schemas/
  memory: memory/
  state: state/
  reports: reports/
  runner: runner/
  bindings: bindings/
  projections: bindings/projections/

writable_targets:
  - memory/**
  - skills/**
  - state/**
  - reports/**

protected_targets:
  - INSTALL.md
  - GUIDELINE.md
  - harness.yaml

risk_policy:
  default_mode: proposal
  auto_apply_allowed:
    - reports/**
    - state/usage.json
  human_approval_required:
    - GUIDELINE.md
    - INSTALL.md
    - hooks/**
    - eval/**

upgrade:
  preserve:
    - memory/**
    - state/usage.json
    - state/pins.json
    - reports/**
    - archive/**
  migration_report: reports/install/
```

## `INSTALL.md`

`INSTALL.md` 是给 host agent 读的说明。它应包含：

```text
# INSTALL.md

## Goal
Install this harness without taking over the host agent runtime.

## Host detection
How to detect supported hosts and capability level.

## Install plan
What files are copied/linked/merged.

## Hook mapping
How recall/observe/reflect/curate map to host lifecycle events.

## Permissions
Writable targets, protected targets, approval rules.

## Fallbacks
Skill-only, manual review, proposal-only modes.
Optional maintenance runner when host lacks scheduler but user opts in.

Runner install rules:

- disabled by default;
- installed only after L2/L3 artifacts are present;
- can be configured as host scheduler, external cron, CLI tick, or resident wrapper;
- resident wrapper must be semantically equivalent to `runner tick`;
- uninstalling runner keeps memory, reports, and state;
- LLM jobs require an approved host command and otherwise downgrade to manual/proposal-only.

## Verify
Dry-run, smoke test, report location.

## Upgrade
Idempotency, schema migration, preservation rules.

## Uninstall
Remove harness bindings without deleting user memory/archive/reports.
```

## Per-Host Install Maps

Host maps live under `install/hosts/*.yaml`.

Host maps should express projection, not just file copying:

```yaml
projection:
  canonical_root: .mnemon
  instruction_mode: managed_block
  skill_mode: symlink_or_copy
  hook_mode: managed_config_patch
  drift_policy: report_before_overwrite
```

Installer must preserve host-owned content outside managed markers. Existing native skills or instructions can be imported only as protected `user + native_import` artifacts unless the user approves a different policy.

### Claude Code

```yaml
host: claude-code
detect:
  commands: ["claude"]
  files_any: ["CLAUDE.md", ".claude/"]
capability:
  max_level: L3
instructions:
  targets:
    - CLAUDE.md
    - .claude/CLAUDE.md
  mode: managed_block
skills:
  targets:
    - .claude/skills/
  mode: symlink_or_copy
hooks:
  recall:
    - SessionStart
    - UserPromptSubmit
  observe:
    - PreToolUse
    - PostToolUse
  reflect:
    - Stop
    - SessionEnd
  curate:
    - scheduled
fallbacks:
  no_hooks: L1
projection:
  canonical_root: .mnemon
  instruction_mode: pointer_block
  skill_mode: symlink_or_copy
  drift_policy: report_before_overwrite
```

### Codex

```yaml
host: codex
detect:
  files_any: ["AGENTS.md", ".codex/"]
capability:
  max_level: L1
instructions:
  targets:
    - AGENTS.md
  mode: managed_block
skills:
  targets:
    - docs/agent-skills/
    - skills/
  mode: pointer_or_copy
hooks:
  recall: ["manual"]
  observe: ["manual"]
  reflect: ["manual"]
  curate: ["manual"]
fallbacks:
  default: L1
projection:
  canonical_root: .mnemon
  instruction_mode: pointer_block
  skill_mode: pointer
  drift_policy: report_before_overwrite
```

### Hermes

```yaml
host: hermes
detect:
  commands: ["hermes"]
  dirs_any: ["~/.hermes/skills"]
capability:
  max_level: L4
instructions:
  targets:
    - "~/.hermes/context/"
  mode: pointer_or_import
skills:
  targets:
    - "~/.hermes/skills/"
  mode: native_import_or_symlink
hooks:
  recall:
    - on_session_start
    - pre_llm_call
  observe:
    - pre_tool_call
    - post_tool_call
  reflect:
    - post_llm_call
    - on_session_end
  curate:
    - curator
    - cron
projection:
  canonical_root: .mnemon
  instruction_mode: pointer
  skill_mode: native_import_or_symlink
  drift_policy: report_before_overwrite
```

### Cursor / Continue / Generic

Cursor and Continue are mainly rule/context surfaces. They can install L0/L1 by default and L2 only when project scripts or external automation are available.

```yaml
host: generic
detect:
  default: true
capability:
  max_level: L0
instructions:
  targets:
    - AGENTS.md
    - README.md
    - .agent-instructions.md
skills:
  targets:
    - skills/
hooks:
  recall: ["manual"]
  observe: ["manual"]
  reflect: ["manual"]
  curate: ["manual"]
```

## Idempotency

Installation must write markers:

```yaml
install:
  harness_version: 0.1.0
  installed_at: "2026-05-08T00:00:00Z"
  host: claude-code
  capability_level: L2
  canonical_root: .mnemon
  installed_files: []
  merged_instruction_blocks:
    - target: CLAUDE.md
      marker: "<!-- self-evolution-harness:start -->"
  hook_bindings: []
  projections: []
```

Rules:

- If marker exists, update in place.
- If user changed generated block, preserve and write conflict report.
- Projection writes are recorded in `bindings/active.json`.
- Drift in projected files writes `reports/projection/` before overwrite.
- Never delete `memory/`, `reports/`, `archive/`, `state/usage.json`, `state/pins.json`.
- Upgrade may migrate schemas, but must write `reports/install/<timestamp>.md`.
- Uninstall removes host bindings and generated skill/hook copies only; user data stays.

## Install Skill Contract

`skills/install/SKILL.md` should instruct the host agent to:

1. Read `harness.yaml`.
2. Detect host.
3. Produce an install plan.
4. Ask approval before modifying host config.
5. Apply only marked blocks and generated files.
6. Run verification.
7. Write install report.

Output schema:

```yaml
type: install_report
host: claude-code
capability_level: L2
actions:
  - target: CLAUDE.md
    action: merge_block
    status: applied
  - target: .claude/skills/
    action: copy
    status: applied
warnings: []
next_steps: []
```
