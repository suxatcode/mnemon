# 10. Filesystem And Hook Mounting

The harness has no mandatory runtime, but it still needs a durable filesystem. Without a canonical filesystem, memory, skills, provenance, reports, projections, and rollback state scatter across host-specific files and become impossible to curate safely.

The recommended design is:

```text
.mnemon/ is canonical.
Host-native files are pointers, projections, or hook bindings.
Host-owned content remains host-owned.
```

This is better than writing directly into every host's native template as the primary state. Native embedding is still required, but installation should be a small hook-and-pointer mounting layer.

## Filesystem References

Existing agent systems are useful references for filesystem design, not for product shape.

| Reference pattern | Harness abstraction |
|---|---|
| Small bounded `MEMORY.md` / `USER.md` | canonical Prompt Memory files with strict budgets |
| `skills/<name>/SKILL.md` with frontmatter | directory-based skill artifacts and schema validation |
| usage/provenance sidecar | engineering metadata outside model-facing Markdown |
| curator reports and backups | report-first maintenance and rollback |
| hooks/cron as lifecycle surface | semantic hook bindings and optional runner jobs |

The part we should not copy is a single host-specific home directory as the only install target. Mnemon should be repo/project-local by default, with optional user/global overlays later.

## Hook-First Mounting

The default path is not a host adapter. The default path is an agent-readable hook contract:

```text
INSTALL.md
  -> host agent identifies instruction / skill / hook / scheduler surfaces
  -> host agent maps recall / observe / reflect / curate
  -> host agent records the binding in .mnemon/bindings/active.json
```

There are two execution styles:

| Style | Description | Boundary |
|---|---|---|
| Agent-executed install | the host agent reads `INSTALL.md` and performs the binding with user approval | primary path |
| Scripted install | a script automates the same plan, approvals, binding record, and smoke tests | later convenience |

Both styles produce the same result: `.mnemon` remains canonical, and host-native surfaces only point to it or invoke semantic hooks.

Native-only installation remains an L0 fallback when the host cannot reference files or register hooks, but it is not the main architecture.

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
    projections/
      <host-label>/
  skills/
    core/
      recall/SKILL.md
      reflect/SKILL.md
      curate/SKILL.md
    project/
    generated/
    archive/
  memory/
    prompt/
      MEMORY.md
      USER.md
      project.md
    longterm/
      episodic/
        evidence/
        transcripts/
        events/
      semantic/
        facts/
        summaries/
        topics/
        index/
      imports/
      archive/
        prompt/
    consolidation/
      candidates/
      promotions/
      demotions/
      decisions/
  hooks/
    recall.md
    observe.md
    reflect.md
    curate.md
  prompts/
  schemas/
  scripts/
  state/
    install.json
    usage.json
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

`fs.yaml` defines the filesystem contract. `inventory.json` records what the installing agent detected in the host project. `bindings/active.json` records which instruction pointers, skill surfaces, and semantic hooks are currently mounted.

## Filesystem Tiers

| Tier | Authority | Examples |
|---|---|---|
| Canonical harness state | `.mnemon` | memory, skills, usage/provenance sidecar, reports, runner jobs |
| Managed bindings | generated from `.mnemon` | marked instruction pointers, skill projections, hook config |
| Host-owned native content | host/user | existing instructions, user rules, native skills outside markers |

Only the first tier is the harness source of truth. The second tier can be regenerated. The third tier must be sensed and respected, not overwritten.

## Host Surface Sensing

Because the harness is mounted on a host agent, installation must detect capabilities rather than assume a product. The installing agent asks: what surfaces can this host expose safely?

Surface sensing reads:

- persistent instruction surfaces;
- native skill or command discovery surfaces;
- lifecycle, model, tool, approval, stop, and session hooks;
- scheduler, cron, idle task, or CI surfaces;
- write permission and approval boundaries;
- existing managed markers from previous installs.

Binding example:

```yaml
host_label: detected-by-agent
capability_level: L2
instruction_surface:
  path: AGENTS.md
  mode: managed_pointer
skill_surface:
  mode: native|pointer|manual
semantic_hooks:
  recall:
    trigger: user_prompt
    target: .mnemon/hooks/recall.md
  observe:
    trigger: post_tool_call
    target: .mnemon/hooks/observe.md
  reflect:
    trigger: session_end
    target: .mnemon/hooks/reflect.md
  curate:
    trigger: manual
    target: .mnemon/skills/core/curate/SKILL.md
```

The installer, whether agent-executed or scripted, should produce an install plan before modifying anything.

## Projection Modes

| Mode | Use case | Behavior |
|---|---|---|
| `pointer` | host can read referenced files | native file points to `.mnemon/GUIDELINE.md`, Prompt Memory, skill index |
| `managed_block` | instruction file supports plain Markdown | insert a small marked block, keep user content untouched |
| `hook_binding` | host supports lifecycle or tool hooks | bind a host event to `.mnemon/hooks/<name>.md` or a core skill |
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
Map host lifecycle events to `.mnemon/hooks/recall.md`, `.mnemon/hooks/observe.md`, `.mnemon/hooks/reflect.md`, and `.mnemon/hooks/curate.md` when hooks are available.
Use `.mnemon/skills/core/*/SKILL.md` as the manual fallback.
Prompt Memory lives under `.mnemon/memory/prompt/`; reports live under `.mnemon/reports/`.
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
.mnemon/skills/generated/dev-server/SKILL.md
```

Projection:

```text
.claude/skills/dev-server/SKILL.md -> .mnemon/skills/generated/dev-server/SKILL.md
```

If symlink is not supported, copy with projection metadata:

```yaml
projection:
  source: .mnemon/skills/generated/dev-server/SKILL.md
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
  read INSTALL.md
  inventory instruction / skill / hook / scheduler surfaces
  create/update .mnemon canonical files
  create hook mounting plan
  ask approval
  write managed pointers / skill projections / hook bindings
  record bindings/active.json
  write install report

runtime:
  host reads native instruction block
  host follows pointers into .mnemon
  host events invoke recall / observe / reflect / curate
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
  hook_binding_mode: host_native_or_manual
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

1. one place for usage/provenance state;
2. host-independent hook binding records, backup, rollback, and reports;
3. stable Prompt/Long-Term Memory layout and explicit consolidation artifacts;
4. safe curator/dreaming over self-authored assets;
5. clean uninstall and upgrade;
6. multi-host portability without a host-specific adapter.

Pure host-native embedding is attractive for first-use ergonomics, but it makes long-term self-evolution fragmented. The right compromise is canonical filesystem plus agent-readable hook mounting.

## Acceptance Criteria

Filesystem design is acceptable when:

1. deleting projections does not delete canonical memory or reports;
2. uninstall removes host bindings without losing `.mnemon`;
3. host files outside managed markers are untouched;
4. projection drift is reported before overwrite;
5. recall/observe/reflect/curate can be mounted as hooks or manual skills;
6. native-only install remains possible as L0 fallback;
7. curator operates on canonical files, not random host templates;
8. every projected artifact points back to its canonical source.
