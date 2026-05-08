# 10. Filesystem And Host Projection

The harness has no mandatory runtime, but it still needs a durable filesystem. Without a canonical filesystem, memory, skills, provenance, reports, projections, and rollback state scatter across host-specific files and become impossible to curate safely.

The recommended design is:

```text
.mnemon/ is canonical.
Host-native files are projections or bindings.
Host-owned content remains host-owned.
```

This is better than writing directly into every host's native template as the primary state. Native embedding is still required, but it should be a projection layer.

## Hermes Lessons

Hermes is worth referencing for filesystem design, not for product shape.

| Hermes pattern | Harness abstraction |
|---|---|
| Small bounded `MEMORY.md` / `USER.md` | canonical hot memory files with strict budgets |
| `skills/<name>/SKILL.md` with frontmatter | directory-based skill artifacts and schema validation |
| usage/provenance sidecar | engineering metadata outside model-facing Markdown |
| curator reports and backups | report-first maintenance and rollback |
| hooks/cron as lifecycle surface | host bindings and optional runner jobs |

The part we should not copy is a single host-specific home directory such as `~/.hermes` as the only install target. Mnemon should be repo/project-local by default, with optional user/global overlays later.

## Two Installation Paths

There are two plausible paths:

| Path | Description | Problem |
|---|---|---|
| Host-native primary | write directly into `CLAUDE.md`, `AGENTS.md`, `.claude/skills`, `~/.hermes/skills`, etc. | portable state, provenance, curation, backup, and uninstall become host-specific |
| Canonical `.mnemon` + projection | keep source of truth in `.mnemon`, mount/project into host-native surfaces | requires a projection layer, but keeps the harness coherent |

The second path is better as the default. It gives the harness its own durable object model without owning runtime execution.

The first path remains useful as an L0/L1 fallback when a host cannot reference files, cannot register skills, or the user explicitly wants a native-only install.

## Canonical Layout

Recommended repo-local install:

```text
.mnemon/
  harness.yaml
  INSTALL.md
  GUIDELINE.md
  fs.yaml
  inventory.json
  bindings/
    active.json
    hosts/
      claude-code.yaml
      codex.yaml
      hermes.yaml
      generic.yaml
    projections/
      claude-code/
      codex/
      hermes/
  skills/
    core/
      recall/SKILL.md
      reflect/SKILL.md
      curate/SKILL.md
    project/
    generated/
      active/
      quarantine/
      candidates/
    archive/
  memory/
    hot/
      MEMORY.md
      USER.md
      project.md
    warm/
      topics/
      sessions/
      candidates/
    cold/
      evidence/
      transcripts/
      imports/
      archive/
      index/
  hooks/
    templates/
    installed/
  prompts/
  schemas/
  scripts/
  state/
    install.json
    usage.json
    pins.json
    lineage.json
    host_activity.json
    jobs/
    locks/
  reports/
    install/
    reflection/
    curator/
    dreaming/
    projection/
    eval/
  backups/
  runner/
    jobs/
    budgets/
```

`fs.yaml` defines the filesystem contract. `inventory.json` records what the installer detected in the host project. `bindings/active.json` records which projections are currently installed.

## Filesystem Tiers

| Tier | Authority | Examples |
|---|---|---|
| Canonical harness state | `.mnemon` | memory, skills, usage, lineage, reports, runner jobs |
| Managed projections | generated from `.mnemon` | marked blocks in `CLAUDE.md`/`AGENTS.md`, copied skill folders, hook config |
| Host-owned native content | host/user | existing instructions, user rules, native skills outside markers |

Only the first tier is the harness source of truth. The second tier can be regenerated. The third tier must be sensed and respected, not overwritten.

## Host Template Sensing

Because the harness is mounted on a host agent, installation must detect and adapt to existing templates instead of blindly writing a new one.

Template sensing reads:

- instruction files: `CLAUDE.md`, `AGENTS.md`, `.cursor/rules`, `continue` config, Hermes config;
- native skill directories;
- hook config files;
- scheduler/cron config;
- existing managed markers from previous installs;
- project conventions such as docs directory, package manager, test commands.

Host map example:

```yaml
host: claude-code
detect:
  files_any:
    - CLAUDE.md
    - .claude/
instruction_surfaces:
  - path: CLAUDE.md
    mode: managed_block
    marker: mnemon
skill_surfaces:
  - path: .claude/skills
    mode: symlink_or_copy
hook_surfaces:
  - path: .claude/settings.json
    mode: managed_json_patch
projection:
  default_mode: pointer
  refresh_after:
    - install
    - curate_apply
    - skill_promote
```

The installer should produce an install plan before modifying anything.

## Projection Modes

| Mode | Use case | Behavior |
|---|---|---|
| `pointer` | host can read referenced files | native file points to `.mnemon/GUIDELINE.md`, hot memory, skill index |
| `managed_block` | instruction file supports plain Markdown | insert a small marked block, keep user content untouched |
| `symlink` | host skill loader follows symlinks | symlink active `.mnemon` skill dirs into native skill dir |
| `copy` | host requires physical files | copy generated projections with checksum and source pointer |
| `json_patch` | host has structured config | apply reversible managed patch |
| `native_import` | user has existing native assets | import into `.mnemon` as user/foreground with protected provenance |

Projection should prefer `pointer` when the host can follow file references. Large memory/skill bodies should not be duplicated into instruction files.

## Managed Blocks

Instruction files should receive a short managed block:

```markdown
<!-- mnemon:start -->
Mnemon self-evolution harness is installed for this project.

Read `.mnemon/GUIDELINE.md` before applying durable memory or skill changes.
Use `.mnemon/skills/core/recall/SKILL.md` for recall, `.mnemon/skills/core/reflect/SKILL.md` after completed work, and `.mnemon/skills/core/curate/SKILL.md` for maintenance.
Hot memory lives under `.mnemon/memory/hot/`; reports live under `.mnemon/reports/`.
Do not edit generated projections directly; update `.mnemon` canonical files.
<!-- mnemon:end -->
```

Rules:

- managed blocks are short;
- blocks point to canonical files instead of copying them;
- content outside markers is user-owned;
- changes inside markers can be regenerated after approval;
- if a user manually edits a managed block, installer records drift before replacing it.

## Native Skill Projection

Canonical skill:

```text
.mnemon/skills/generated/active/dev-server/SKILL.md
```

Projection:

```text
.claude/skills/dev-server/SKILL.md -> .mnemon/skills/generated/active/dev-server/SKILL.md
```

If symlink is not supported, copy with projection metadata:

```yaml
projection:
  source: .mnemon/skills/generated/active/dev-server/SKILL.md
  target: .claude/skills/dev-server/SKILL.md
  checksum: sha256:...
  mode: copy
  generated_at: 2026-05-08T00:00:00Z
```

Direct edits to projected copies are drift. The installer should preserve them as conflict reports or offer explicit import.

## Host-Native Import

Existing native instructions and skills should be imported only when useful:

```text
host native skill
  -> import report
  -> .mnemon/skills/project/<name>/SKILL.md
  -> provenance: user + native_import
  -> protected by default
```

Import is not automatic mutation. It is a read/normalize/propose operation unless the user approves.

## Conflict Policy

| Conflict | Resolution |
|---|---|
| user changes outside managed block | keep user content |
| user changes inside managed block | write projection drift report before replacing |
| canonical file changed and projection stale | regenerate projection |
| projected copy changed manually | preserve as conflict artifact; propose import or overwrite |
| host native asset conflicts with canonical generated skill | canonical remains source; native asset is imported/protected if approved |
| two hosts project the same skill differently | host-specific projection metadata records divergence |

The harness should never silently choose host-native state over canonical state.

## Mount Lifecycle

```text
install:
  detect host templates
  inventory native surfaces
  create/update .mnemon canonical files
  create projection plan
  ask approval
  write managed blocks / symlinks / copies / hook bindings
  record bindings/active.json
  write install report

runtime:
  host reads native instruction block
  host follows pointers into .mnemon
  hooks call .mnemon skills/prompts/scripts
  reports and sidecars are written in .mnemon

maintenance:
  curator/dreaming updates canonical files
  projection refresh runs after apply
  drift is detected and reported

uninstall:
  remove managed blocks and generated projections
  keep .mnemon memory/state/reports/backups unless user requests deletion
```

## `fs.yaml`

`fs.yaml` is the machine-readable filesystem policy.

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
  memory_hot: memory/hot
  memory_warm: memory/warm
  memory_cold: memory/cold
  skills_active:
    - skills/core
    - skills/project
    - skills/generated/active
  skills_quarantine: skills/generated/quarantine
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

## Why This Is Better

Canonical `.mnemon` is better because it gives the harness:

1. one place for usage/provenance/lineage;
2. host-independent backup, rollback, and reports;
3. stable hot/warm/cold memory layout;
4. safe curator/dreaming over self-authored assets;
5. clean uninstall and upgrade;
6. multi-host portability.

Pure host-native embedding is attractive for first-use ergonomics, but it makes long-term self-evolution fragmented. The right compromise is canonical filesystem plus host-native projection.

## Acceptance Criteria

Filesystem design is acceptable when:

1. deleting projections does not delete canonical memory or reports;
2. uninstall removes host bindings without losing `.mnemon`;
3. host files outside managed markers are untouched;
4. projection drift is reported before overwrite;
5. native-only install remains possible as L0 fallback;
6. curator operates on canonical files, not random host templates;
7. every projected artifact points back to its canonical source.
