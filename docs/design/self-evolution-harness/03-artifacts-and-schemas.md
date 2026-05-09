# 03. Artifacts 与 Schemas

本设计中的 schema 是契约，不要求所有 host 使用同一种实现。Host 可以用 JSON Schema、YAML 校验、脚本校验或人工 review，但字段语义应一致。

## Filesystem Manifest

`fs.yaml` defines `.mnemon` canonical filesystem policy and host projection behavior:

```yaml
schema_version: 1
root: .mnemon
authority: canonical
protected:
  - GUIDELINE.md
  - INSTALL.md
  - harness.yaml
  - schemas/**
  - hooks/**
canonical:
  memory_prompt: memory/prompt
  memory_longterm: memory/longterm
  memory_consolidation: memory/consolidation
  skills_active:
    - skills/core
    - skills/project
    - skills/generated
  skills_archive: skills/archive
  reports: reports
projection:
  managed_marker: mnemon
  default_mode: pointer
  refresh_events:
    - install
    - upgrade
    - curate_apply
    - skill_promote
drift:
  action: report
  report_dir: reports/projection
```

`bindings/active.json` records installed projections:

```json
{
  "schema_version": 1,
  "host": "claude-code",
  "canonical_root": ".mnemon",
  "projections": [
    {
      "id": "claude-instruction",
      "source": ".mnemon/GUIDELINE.md",
      "target": "CLAUDE.md",
      "mode": "managed_block",
      "marker": "mnemon",
      "checksum": "sha256:..."
    }
  ]
}
```

Projection state is regenerable. Canonical state is not.

## Skill Artifact

每个 skill 是一个目录：

```text
skills/<category>/<name>/
  SKILL.md
  references/
  templates/
  scripts/
  assets/
```

Recommended categories:

- `skills/core/`: harness-provided package skills.
- `skills/project/`: user/project-authored skills, protected by default.
- `skills/generated/`: agent-authored skills; lifecycle state lives in `state/usage.json`.
- `skills/archive/`: archived skill artifacts.

`SKILL.md` frontmatter：

```yaml
---
name: reflect
description: Review completed work and propose durable memory or skill updates.
---
```

字段：

| Field | Required | Meaning |
|---|---:|---|
| `name` | yes | stable skill id |
| `description` | yes | discovery text |
| `version` | no | package version |

Governance fields such as `created_by`, `provenance`, `state`, and `pinned` belong in `state/usage.json`, following the sidecar pattern.

Rules:

- Prefer patching existing class-level skill.
- Use support files for long examples.
- Do not create one-session-one-skill.
- Package/harness skills are not auto-curated.

## Prompt Memory Artifact

Prompt Memory is the engineering implementation of Working Memory. It is small Markdown:

```text
memory/prompt/
  MEMORY.md
  USER.md
  project.md
```

Recommended budgets:

| File | Target |
|---|---:|
| `MEMORY.md` | 2k-4k chars |
| `USER.md` | 1k-2k chars |
| `project.md` | 2k-6k chars |

Prompt Memory is fully loaded into the host prompt snapshot. It is not a recall database.

Entry shape:

```markdown
§
type: preference
source: user-confirmed
updated: 2026-05-08
risk: low

User prefers concise technical summaries after implementation.
```

Rules:

- Facts/preferences only.
- Declarative, not imperative.
- Current user request overrides memory.
- Exceeding budget produces a consolidation/demotion proposal, not silent truncation.

## Usage Sidecar

`state/usage.json`:

```json
{
  "schema_version": 1,
  "skills": {
    "reflect": {
      "created_by": "harness",
      "provenance": "package",
      "state": "active",
      "pinned": true,
      "view_count": 0,
      "use_count": 0,
      "patch_count": 0,
      "created_at": "2026-05-08T00:00:00Z",
      "last_used_at": null,
      "last_patched_at": null,
      "absorbed_into": null,
      "archived_at": null
    }
  }
}
```

Auto-curation eligibility:

```text
created_by == "agent"
AND provenance in {"background_review", "curator"}
AND pinned != true
AND state in {"active", "stale"}
AND target not protected
```

User, package, harness, imported, and pinned artifacts default to no auto mutation.

## Long-Term Memory And Consolidation Artifacts

Long-Term Memory is split by cognitive role. Mnemon Store carries episodic and semantic memory; skills carry procedural memory.

```text
memory/longterm/
  episodic/
    evidence/
    transcripts/
    events/
    decisions/
    failures/
  semantic/
    facts/
    preferences/
    summaries/
    topics/
    index/
  archive/
    prompt/
  imports/

memory/consolidation/
  candidates/
  summaries/
  promotions/
  demotions/
  decisions/
```

Consolidation artifacts are staging records for Prompt Memory / Long-Term Memory movement, not a third memory layer.

Promotion proposal:

```yaml
type: prompt_promotion
from:
  longterm_refs:
    - memory/longterm/semantic/summaries/session-2026-05-09.md
candidate: memory/consolidation/candidates/build-tooling.yaml
to: memory/prompt/project.md
scores:
  importance: 0.86
  confidence: 0.91
  recurrence: 0.74
  risk: 0.12
patch:
  action: add_or_replace
  content: "This repo uses pnpm for frontend package management."
```

Demotion proposal:

```yaml
type: prompt_demotion
from: memory/prompt/project.md
to:
  longterm_ref: memory/longterm/archive/prompt/project-2026-05-09.md
reason: "Too detailed for always-on prompt memory."
replacement:
  prompt_pointer: "Build details archived in long-term memory; recall when working on frontend tooling."
```

## Hook IO

Base input:

```yaml
event: pre_llm_call
host: claude-code
capability_level: L2
hook_id: recall.pre_llm
idempotency_key: session-123:pre_llm_call:001
session_id: string
cwd: string
timestamp: string
payload: {}
budgets:
  max_output_chars: 1500
  timeout_ms: 800
  write_allowed: false
```

Hook output envelope:

```yaml
hook_id: recall.pre_llm
idempotency_key: session-123:pre_llm_call:001
status: ok|none|skipped|proposal|error|timeout
latency_ms: 120
retryable: false
writes: []
warnings: []
errors: []
```

Hook contract rules:

- `idempotency_key` must make retries safe;
- latency budget is part of the hook input;
- timeout means no mutation unless an earlier idempotent write is already recorded;
- `none` is a successful empty result, not an error;
- hooks must declare whether they can write before execution;
- status is always reportable in later reflection/curator jobs.

Recall output:

```yaml
type: recall
status: ok
context:
  - source: memory/prompt/project.md
    confidence: high
    text: "Use pnpm for this repository."
warnings: []
```

No recall:

```yaml
type: recall
status: none
reason: "No relevant memory above threshold."
```

Reflection output:

```yaml
type: reflection
mode: proposal
proposals:
  - id: refl-001
    target: skills/debugging/SKILL.md
    action: patch
    risk: low
    reason: "Repeated dev-server port collision workaround succeeded."
    evidence:
      - reports/reflection/2026-05-08.md
    patch:
      type: append_section
      content: "..."
```

Curator output:

```yaml
type: curator
mode: dry-run
consolidations:
  - from: debug-vite-port
    into: dev-server-troubleshooting
    reason: "Covered by umbrella skill."
archives:
  - target: stale-release-checklist
    reason: "Unused and superseded."
```

## Write Target Allowlist

`schemas/write-target-allowlist.json` expresses install-time write policy:

```json
{
  "allow": [
    "memory/**",
    "skills/**",
    "state/**",
    "reports/**",
    "archive/**"
  ],
  "protect": [
    "INSTALL.md",
    "GUIDELINE.md",
    "harness.yaml",
    "hooks/**",
    "eval/**",
    "schemas/**"
  ],
  "approval_required": [
    "GUIDELINE.md",
    "INSTALL.md",
    "harness.yaml",
    "hooks/**",
    "eval/**"
  ],
  "hardline_block": [
    "host_config_outside_marker",
    "secret_exfiltration",
    "destructive_filesystem_operation",
    "safety_policy_weakening"
  ]
}
```

If host cannot enforce this allowlist, reflection and curator must run proposal-only. Risk classification follows the R0-R4 model in `05-memory-curation-eval.md`.

Minimal risk result:

```yaml
risk:
  level: R0|R1|R2|R3|R4
  source: user|agent|background_review|curator|imported|package
  verdict: safe|caution|dangerous
  decision: allow|proposal|approval_required|block
  reasons: []
  required_gates:
    - target-allowlist
    - schema-validation
    - static-scan
    - report-written
```

## Reports

All maintenance writes reports. Report metadata:

```yaml
report:
  id: string
  type: install|reflection|curator|dreaming|eval|migration|skill-production
  host: string
  capability_level: string
  started_at: string
  finished_at: string
  mode: dry-run|proposal|apply
  summary: string
  actions: []
  warnings: []
  errors: []
  evidence: []
```

Report files:

```text
reports/
  install/<timestamp>.md
  reflection/<timestamp>.md
  curator/<timestamp>.md
  eval/<timestamp>.md
```

## Maintenance Runner Jobs

Maintenance runner jobs are optional artifacts. Host scheduler, external cron, or the optional runner can execute them.

```text
runner/
  jobs/
    reflection.yaml
    curator.yaml
    dreaming.yaml
    index.yaml
    eval.yaml
  locks/
  budgets/
```

Job descriptor:

```yaml
job:
  id: curator-weekly
  type: curator
  enabled: false
  trigger:
    kind: idle_or_schedule
    interval_hours: 168
    min_idle_minutes: 30
  mode: dry-run
  inputs:
    - state/usage.json
    - skills/**
    - memory/prompt/**
    - memory/longterm/semantic/summaries/**
    - memory/consolidation/**
  write_allowlist:
    - reports/curator/**
    - memory/consolidation/**
    - state/curator_state.json
  budgets:
    max_runtime_seconds: 900
    max_llm_calls: 8
    max_output_chars: 20000
  locking:
    key: curator
    stale_after_seconds: 3600
  kill_switch:
    file: state/maintenance_disabled
```

Runner job types:

| Type | Purpose | Default mode |
|---|---|---|
| `reflect.deferred` | delayed post-turn review when host cannot run immediate hook | proposal |
| `curator.transitions` | deterministic usage state updates | apply to state only |
| `curator.review` | skill/memory consolidation, demotion, archive proposals | dry-run |
| `dreaming.light` | extract candidates from long-term evidence and summaries | consolidation candidate write |
| `dreaming.rem` | consolidate themes and write dreaming report | report-only |
| `dreaming.deep` | promotion/demotion proposals from scored candidates | proposal |
| `longterm.index.incremental` | update long-term memory search index | apply to index only |
| `longterm.index.rebuild` | rebuild long-term memory FTS/vector/index artifacts | apply to index only |
| `eval.batch` | run constraints/eval and write PR proposal | proposal |
| `snapshot.rotate` | maintain backup retention | apply |

Job ledger entry:

```json
{
  "schema_version": 1,
  "job_id": "curator-weekly",
  "job_type": "curator.review",
  "status": "proposal_written",
  "mode": "dry-run",
  "started_at": "2026-05-08T00:00:00Z",
  "finished_at": "2026-05-08T00:02:00Z",
  "inputs": ["state/usage.json", "skills/**"],
  "outputs": ["reports/curator/2026-05-08.md"],
  "mutations": [],
  "warnings": []
}
```

LLM-based jobs must call a declared host command. The runner must not embed a separate model SDK or tool router.

## Backup Policy

Backup before mutating:

- `skills/**`
- `memory/prompt/**`
- `memory/consolidation/**`
- `state/usage.json`

Backup manifest:

```yaml
backup:
  id: string
  reason: pre-curator-apply
  created_at: string
  files: []
  report: reports/curator/...
```
