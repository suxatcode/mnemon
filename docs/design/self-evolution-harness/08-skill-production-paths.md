# 08. Skill Index And Manage

Mnemon should keep the skill system deliberately small. The harness skill loop is an agent-agnostic contract:

```text
skills_list / skill_view
  -> skill_manage
  -> usage sidecar
  -> background review
  -> curator
```

The host agent still owns the runtime, model loop, tools, UI, and permissions. Mnemon owns the canonical filesystem, schemas, reports, and projection contract.

## Skill Loop Shape

The useful shape is:

| Mechanism | Harness abstraction |
|---|---|
| `skills_list` | metadata-only skill index |
| `skill_view(name[, file_path])` | progressive disclosure for `SKILL.md` and support files |
| `skill_manage` | create/edit/patch/delete/write_file/remove_file contract |
| `SKILL.md` frontmatter | `name` + `description` for discovery |
| support dirs | `references/`, `templates/`, `scripts/`, `assets/` |
| `.usage.json` | usage, provenance, lifecycle state, pinned flag |
| background review fork | post-turn `reflect` hook/job |
| curator | scheduled/idle/manual `curate` hook/job |
| class-level skill policy | patch umbrella skills before creating narrow skills |

The only translation is runtime binding. Mnemon exposes the same semantics through host skills, hooks, CLI commands, or queued jobs.

## Skill Artifact

Each skill is a directory:

```text
skills/<namespace>/<name>/
  SKILL.md
  references/
  templates/
  scripts/
  assets/
```

Recommended harness layout:

```text
.mnemon/
  skills/
    core/
      install/SKILL.md
      recall/SKILL.md
      observe/SKILL.md
      reflect/SKILL.md
      curate/SKILL.md
    project/
    generated/
    archive/
  state/
    usage.json
    curator_state.json
  reports/
    reflection/
    curator/
```

This intentionally stays closer to a small managed skill library than a multi-stage generated skill tree. Agent-created skills live under `skills/generated/`; their state is in `state/usage.json`. Archived skills move to `skills/archive/`.

`SKILL.md` frontmatter should stay small:

```yaml
---
name: debug-build-failures
description: Diagnose recurring build failures by checking environment, dependency, cache, and test signals.
---
```

Rules:

- `name` is stable, lowercase, filesystem-safe, and class-level.
- `description` is the discovery string; it should tell the model when to load the skill.
- Operational state does not live in frontmatter.
- Long session detail moves to `references/`.
- Reusable starter files move to `templates/`.
- Deterministic checks move to `scripts/`.
- Binary or media assets move to `assets/`.

## Skill Index

The index is progressive disclosure:

```text
list skills
  -> name, description, namespace/state summary
view skill
  -> full SKILL.md
view support file
  -> references/*, templates/*, scripts/*, assets/*
```

The index should be cheap enough to load during review. Full skill bodies and support files are read only when relevant.

`skills_list` equivalent:

```yaml
input:
  namespace: optional
output:
  skills:
    - name: string
      description: string
      namespace: core|project|generated
      state: active|stale|archived
      pinned: boolean
```

`skill_view` equivalent:

```yaml
input:
  name: string
  file_path: optional
output:
  content: string
  linked_files:
    references: []
    templates: []
    scripts: []
    assets: []
```

## Skill Manage

The write surface should stay compact:

| Action | Meaning | Default policy |
|---|---|---|
| `create` | create a new `SKILL.md` | allowed for foreground-confirmed or background review |
| `patch` | replace a unique string in `SKILL.md` or support file | preferred update path |
| `edit` | rewrite full `SKILL.md` | major overhaul only |
| `write_file` | add/update support file | preferred for long details |
| `remove_file` | remove support file | report required |
| `delete` | remove from active library | harness maps this to archive for recoverability |

The harness should implement deletion as a recoverable archive operation when the target is self-authored. The tool name can still be `delete` for compatibility, but the storage effect should be:

```text
skills/generated/<name> -> skills/archive/<name>
state: archived
archived_at: timestamp
absorbed_into: optional umbrella skill
```

Write rules:

- Patch before edit.
- Patch/edit currently loaded skills first.
- Then patch existing umbrella skills.
- Then write support files under an existing umbrella.
- Create a new skill only if no existing class-level skill covers the behavior.
- Skip simple one-off tasks.
- Confirm with the user before foreground create/delete.
- Every mutation clears host/projection skill cache if the host has one.
- Every mutation records usage sidecar updates and a report.

## Usage Sidecar

Governance state stays outside `SKILL.md`.

```json
{
  "schema_version": 1,
  "skills": {
    "debug-build-failures": {
      "created_by": "agent",
      "provenance": "background_review",
      "state": "active",
      "pinned": false,
      "use_count": 3,
      "view_count": 7,
      "patch_count": 1,
      "created_at": "2026-05-09T00:00:00Z",
      "last_used_at": "2026-05-09T00:00:00Z",
      "last_viewed_at": "2026-05-09T00:00:00Z",
      "last_patched_at": "2026-05-09T00:00:00Z",
      "archived_at": null,
      "absorbed_into": null
    }
  }
}
```

Lifecycle states stay minimal:

```text
active -> stale -> archived
```

`pinned` is orthogonal:

```text
pinned == true
  -> curator skips stale/archive/delete
  -> patch/edit may still be allowed when explicitly requested
```

Auto-curation eligibility:

```text
created_by == "agent"
AND provenance in {"background_review", "curator"}
AND pinned != true
AND state in {"active", "stale"}
AND target not protected
```

User, project, core, imported, and pinned skills are not auto-curated.

## Three Production Entrances

The harness has three practical production entrances.

### 1. User-Declared

The user explicitly asks to save or update a procedure.

```text
user request
  -> inspect skill index
  -> patch existing skill if possible
  -> create only if needed
  -> mark foreground/user-owned
```

Policy:

- protected by default;
- curator does not touch it automatically;
- high-risk policy/hook/install changes require approval.

### 2. Agent-Offered

During foreground work, the agent notices a reusable procedure and asks the user whether to save it.

Trigger examples:

- complex task succeeded after several tool calls;
- errors were overcome;
- user-corrected approach worked;
- non-trivial workflow was discovered;
- user asks to remember a procedure.

Policy:

- no confirmation, no durable write;
- confirmed writes are foreground-owned;
- curator does not silently archive them.

### 3. Background Review

After the answer is delivered, Mnemon represents background review as a host-native post-turn hook or queued `reflect` job.

```text
completed turn
  -> review prompt
  -> classify memory vs skill vs session note
  -> inspect loaded skills
  -> patch existing skill / write support file / create new skill
  -> mark agent-created
```

Review preference order:

1. Update a currently loaded skill.
2. Update an existing umbrella skill.
3. Add a support file under an existing umbrella.
4. Create a new class-level umbrella skill.
5. Say "nothing to save" when no real signal exists.

Background review is the only automatic production path that makes a skill curator-eligible by default.

## Curator Governance

Curator is not a fourth per-turn production entrance. It is the maintenance path that keeps the skill library usable.

Inputs:

- `state/usage.json`;
- active generated skills;
- archived skills;
- reflection reports;
- curator state;
- host/projection inventory.

Actions:

- mark inactive agent-created skills stale;
- archive stale agent-created skills after configured time;
- merge narrow skills into umbrella skills;
- move narrow but useful detail into `references/`, `templates/`, or `scripts/`;
- keep pinned skills untouched;
- write curator reports;
- snapshot before apply.

Curator rules:

- only touches agent-created skills;
- never touches core/project/imported/user-owned skills by default;
- archive over delete;
- skip pinned;
- prefer umbrella skills over one-session skills;
- require `absorbed_into` when one skill is merged into another.

## Memory Interaction

The memory/skill boundary is simple:

```text
memory = who the user is / durable preferences / current operating context
skills = how to do a class of task
```

Mnemon should keep the same boundary:

| Signal | Destination |
|---|---|
| user preference or durable fact | Working Memory / Long-Term Memory |
| reusable workflow or tool tactic | Skill |
| raw logs, traces, failures | episodic Long-Term Memory |
| repeated procedural pattern found during maintenance | skill patch/create through curator or review |

Background review may run as a combined memory+skill review, but the classification stays simple. If a user says "stop formatting answers this way", that can be both a memory preference and a skill patch when it governs a task class.

## Dreaming Interaction

Dreaming should not become a second skill framework. Its role is to surface evidence to the same skill path.

```text
episodic evidence + reports
  -> repeated workflow signal
  -> reflect/curate prompt
  -> skill_manage patch/create/write_file
  -> usage sidecar update
```

Dreaming can feed curator with summaries such as:

- repeated failure recovery path;
- repeated user correction about a workflow;
- recurring command sequence;
- stale or overlapping skill evidence;
- topic cluster suitable for an umbrella skill.

The actual write still goes through `skill_manage` and sidecar rules.

## Harness Binding

Mnemon must not require a resident runtime. The same contract can be bound in several ways:

| Host capability | Binding |
|---|---|
| native tools | expose `skills_list`, `skill_view`, `skill_manage` directly |
| native skills | install `SKILL.md` instructions that call Mnemon CLI/scripts |
| lifecycle hooks | run post-turn `reflect` and scheduled `curate` |
| weak host | write reports/proposals only; user applies manually |
| external cron | run curator/dreaming jobs outside the host session |

The harness-specific responsibility is not to make a new agent. It is to keep:

- canonical skill files;
- usage/provenance sidecar;
- report history;
- host projection metadata;
- reversible archive.

## Acceptance Criteria

The skill system is acceptable when:

1. skill artifacts match the harness shape;
2. index/manage semantics stay compact and host-agnostic;
3. lifecycle is only `active/stale/archived` plus `pinned`;
4. background review-created skills are curator-eligible;
5. foreground user/user-confirmed skills are protected;
6. curator only governs agent-created skills;
7. memory and skill boundaries stay simple;
8. dreaming feeds the same skill_manage path rather than creating a separate pipeline;
9. host projection is derived from `.mnemon`, not a second source of truth;
10. every mutation has sidecar state and report evidence.
