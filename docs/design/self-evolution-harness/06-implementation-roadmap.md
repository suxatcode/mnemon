# 06. Implementation Roadmap

## Phase 0: Spec Package

Goal: create the `.mnemon` canonical filesystem skeleton with no host automation.

Deliverables:

- `harness.yaml`
- `INSTALL.md`
- `GUIDELINE.md`
- `fs.yaml`
- `schemas/`
- `skills/recall`
- `skills/reflect`
- `skills/curate`
- `reports/templates/`

Acceptance:

- A generic agent can read `INSTALL.md` and understand manual L0 installation.
- `GUIDELINE.md` clearly defines memory vs skill.
- `reflect` skill outputs proposal-only reports.
- `.mnemon` can be inspected without any host-native projection.

## Phase 1: L1 Installable Harness

Goal: install into instruction/skill surfaces.

Deliverables:

- `install/hosts/generic.yaml`
- `install/hosts/codex.yaml`
- `install/hosts/claude-code.yaml`
- install skill that generates install plan
- idempotent instruction block markers
- host template sensing
- managed block / pointer projection
- `bindings/active.json`
- `inventory.json`
- `state/install.json`

Acceptance:

- Re-running install does not duplicate blocks.
- Uninstall removes generated bindings but keeps memory/reports/state.
- Upgrade writes migration report.
- Host-owned content outside markers is untouched.

## Phase 2: L2 Hooks

Goal: add recall/observe/reflect hook templates.

Deliverables:

- `hooks/recall/`
- `hooks/observe/`
- `hooks/reflect/`
- `schemas/hook-io.schema.json`
- `schemas/write-target-allowlist.schema.json`
- hook idempotency/status/latency envelope
- `scripts/scan-memory-write`
- `scripts/validate-skill`
- `scripts/check-target-allowlist`

Acceptance:

- Recall can return `NONE`.
- Observe writes cold evidence only.
- Reflect writes proposal reports when allowlist cannot be enforced.
- Low-risk direct patch only happens with enforced allowlist.

## Phase 3a: L3 Curator Skill

Goal: add maintenance governance without owning scheduler or host runtime.

Deliverables:

- `skills/curate`
- `prompts/curator.md`
- `hooks/curate/`
- scheduled descriptors for supported hosts
- `scripts/snapshot`
- `scripts/rollback`
- `state/curator_state.json`
- `state/pins.json`
- `reports/templates/curator.md`
- quarantine/lineage fields in `state/usage.json`

Acceptance:

- Curator dry-run produces structured report.
- Apply mode requires backup.
- Pinned artifacts are skipped.
- Package/harness/imported/user-created artifacts are skipped unless approved.
- Archive is recoverable.

## Phase 3b: Optional Maintenance Runner

Goal: provide cron/lease/ledger execution for asynchronous maintenance without becoming an agent framework.

Deliverables:

- `runner/jobs/curator.yaml`
- `runner/jobs/dreaming.yaml`
- `runner/jobs/reflection.yaml`
- `runner/jobs/index.yaml`
- `schemas/runner-job.schema.json`
- `schemas/job-ledger.schema.json`
- `state/jobs/queue/`
- `state/jobs/done/`
- `state/runner.disabled`
- `scripts/runner-tick` or equivalent thin CLI

Acceptance:

- Runner can be fully disabled while manual skills still work.
- LLM jobs call a configured host command or downgrade to proposal-only.
- Every job attempt writes ledger and report.
- Apply mode requires lease, budget, schema validation, allowlist, and backup.
- Resident daemon and cron invocation have equivalent semantics.
- Foreground host activity can defer expensive maintenance jobs.

## Phase 4: Cold Memory Protocol

Goal: support high-capacity memory without replacing Markdown control plane.

Deliverables:

- `schemas/cold-memory-prefetch.schema.json`
- `schemas/cold-memory-sync.schema.json`
- `prompts/promotion.md`
- warm/cold directory conventions
- recall ranking fields
- cold index descriptor
- explicit `NONE` gate for irrelevant memory

Acceptance:

- Cold memory never injects raw transcripts directly.
- Recall output stays within budget.
- Promotion proposal links evidence.
- Demotion preserves source in warm/cold.

## Phase 5: Eval-Driven Evolution

Goal: evaluate harness artifact changes.

Deliverables:

- `eval/constraints.yaml`
- sample eval dataset schema
- `eval/templates/pr.md`
- report schema for eval result

Acceptance:

- Skill prompt changes run schema + sample eval.
- Hook prompt changes run regression cases.
- Guideline/install map changes require human approval.
- Eval output is proposal/PR, not hot mutation.

## Initial File Tree

First implementation should start with:

```text
.mnemon/
  fs.yaml
  inventory.json
  bindings/
    active.json
  harness.yaml
  INSTALL.md
  GUIDELINE.md
  skills/
    core/
      recall/SKILL.md
      reflect/SKILL.md
      curate/SKILL.md
  schemas/
    skill.schema.json
    usage.schema.json
    proposal.schema.json
    report.schema.json
    write-target-allowlist.schema.json
  reports/
    templates/
      reflection.md
      curator.md
  state/
    install.json
    usage.json
```

Do not start by writing a daemon, server, SDK, database adapter, or universal agent wrapper. Add the optional maintenance runner only after artifact contracts, skills, hooks, reports, and safety model are stable. The runner starts as a tick-style CLI; a resident process is only an equivalent wrapper around the same job semantics.

## Open Decisions

| Decision | Options | Recommendation |
|---|---|---|
| Package root | host-native primary vs repo-local `.mnemon/` | use `.mnemon/` as canonical root, project into host-native files |
| Schema format | JSON Schema vs YAML docs | JSON Schema for machine contracts, Markdown for explanation |
| Direct apply | never vs low-risk allowlisted | allow low-risk only when host enforces write target |
| Host maps | built-in vs community contributed | built-in core maps, allow community maps |
| Cold index | none vs SQLite/FTS/vector | protocol first, implementation later |
| Runner packaging | no runner vs CLI tick vs resident process | CLI tick first; resident process only as equivalent wrapper |
| LLM maintenance | embedded SDK vs host command | host command only; missing command means proposal/manual |
| Projection mode | pointer vs symlink vs copy | pointer first, symlink/copy only for native skill loaders |

## Risks

| Risk | Mitigation |
|---|---|
| Harness becomes hidden agent runtime | no mandatory agent runtime; optional runner is cron/lease/ledger only |
| Host cannot enforce write limits | proposal-only fallback |
| Hot memory grows too much | budget + demotion proposal |
| Skill explosion | class-first guideline + curator |
| User-created artifacts mutated | provenance and created_by gates |
| Install corrupts host config | dry-run, markers, backup, uninstall |
| Host-native files drift from `.mnemon` | projection checksums, drift reports, explicit import |
| Cold recall injects noise | ranking + `NONE` gate + budget |
| Evaluation becomes theater | explicit constraints and held-out cases |
| Runner competes with foreground task | foreground activity signal, leases, budget, deferral |

## Success Criteria

The first usable harness is successful when:

1. It can be installed manually in a generic agent using only Markdown.
2. It can be installed in at least one hook-capable host at L2.
3. It produces reflection proposals after a task.
4. It never patches outside write allowlist.
5. It preserves memory/state/reports across reinstall and upgrade.
6. It can run curator dry-run and produce a useful report.
7. Users can inspect every durable change as a Markdown diff.
