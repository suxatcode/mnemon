# 08. Skill Production Paths

The harness treats skill as the primary unit of self-evolution. Memory stores stable facts, preferences, and compact context. Skills store reusable procedures, operational strategies, tool workflows, and domain tactics. This mirrors the strongest Hermes lesson: self-evolution is less about an engineered memory database and more about repeatedly turning experience into agent-readable behavior assets.

## Core Principle

```text
facts / preferences / stable project context -> memory
procedures / workflows / repeated tactics -> skill
raw evidence / transcript / failed attempts -> cold memory
task continuity -> session summary
```

Skill production must be conservative. A system that creates one skill per turn becomes noisy and harder to use. The default is:

1. patch an existing skill;
2. create an umbrella skill only when a repeated class of work emerges;
3. write a proposal report when evidence is weak;
4. let curator archive or consolidate self-authored skills later.

## Three Production Paths

| Path | Trigger | Producer | Output | Provenance | Auto-curation |
|---|---|---|---|---|---|
| Foreground update | user asks or current task explicitly needs it | active host agent | skill patch/create or proposal | `user` / `foreground` | no by default |
| Post-turn review | `turn_delivered`, `Stop`, `SessionEnd`, queued reflection | host review agent or runner job | memory/skill proposal, optional low-risk patch | `agent` + `reflection` | yes, if self-authored |
| Maintenance synthesis | curator/dreaming/index/eval schedule | curator or dreaming job | umbrella skill, consolidation, archive/promotion proposal | `agent` + `curator` / `dreaming` | yes, within allowlist |

These are architectural paths, not hardcoded implementations. Hermes can implement path 2 with a background review agent. Claude Code can implement path 2 with Stop hooks. Codex can implement it with explicit skill invocation or queued jobs. A generic agent can implement it manually.

## Path A: Foreground Skill Update

Foreground updates are user-directed or task-directed.

Examples:

- user says "把这个流程写成 skill";
- current task requires editing a known skill;
- installer creates the core harness skill pack;
- migration updates package-provided skills.

Rules:

- user-authored content is protected by default;
- foreground changes should preserve the user's intent even if curator later disagrees;
- automatic curator must not rewrite foreground/user skills unless explicitly approved;
- write report if the change affects harness policy, hooks, install map, or guideline.

Foreground provenance:

```yaml
created_by: user|agent|harness
provenance: foreground
curation_policy: protected|manual-review
```

## Path B: Post-Turn Review

Post-turn review is the Hermes-style self-improvement loop. It is triggered after the active task completes, so it can inspect outcomes without competing with the user's current request.

```text
turn summary + tool outcomes + user corrections
  -> reflection prompt
  -> classify insight
  -> choose memory / skill / session / evidence
  -> generate proposal or low-risk patch
  -> validate target and schema
  -> write report
```

Reflection classification:

| Insight | Destination | Example |
|---|---|---|
| stable user preference | hot memory | "User prefers concise technical summaries." |
| project fact | hot/warm memory | "This repo uses pnpm." |
| reusable workflow | skill | "How to recover from Vite port collision." |
| one-off task progress | session summary | "PR review stopped at file X." |
| raw log/error | cold evidence | command output, stack trace |
| uncertain inference | report only | "Likely cause was cache issue." |

Post-turn review can be implemented in three ways:

| Host capability | Implementation |
|---|---|
| Background review agent | fork a restricted review agent after stop |
| Hook-capable host | run `reflect` hook with write allowlist |
| Weak host | enqueue `reflect.deferred` job for runner/manual processing |

Review-agent constraints:

- it receives a summarized transcript or bounded evidence pack;
- it cannot talk to the user;
- it cannot call arbitrary tools;
- it cannot patch protected targets;
- it prefers patching existing skills over creating new skills;
- it writes a report for every proposal or mutation.

## Path C: Maintenance Synthesis

Maintenance synthesis is not about a single turn. It detects patterns across time.

Inputs:

- `state/usage.json`;
- reflection reports;
- curator reports;
- warm candidates;
- cold evidence index;
- active skills;
- pins and protection rules.

Outputs:

- umbrella skill proposals;
- duplicated skill consolidation;
- stale skill archive proposal;
- hot-to-warm demotion;
- warm-to-hot promotion;
- eval/PR proposal for high-risk changes.

This is where dreaming matters. Dreaming turns accumulated low-level evidence into candidates and theme reports. Curator then applies deterministic governance and writes bounded proposals.

## Skill Creation Pipeline

Every path should follow the same pipeline:

```text
observe signal
  -> classify destination
  -> search existing skill index
  -> patch existing skill if enough overlap
  -> create new skill only if class-level behavior exists
  -> assign provenance and curation policy
  -> validate schema / size / protected target
  -> write report
  -> apply or propose
```

Class-level behavior means the skill is likely to help future tasks beyond the exact session that created it.

Creation gates:

| Gate | Requirement |
|---|---|
| Reuse | at least one repeated pattern, user request, or strong project-level workflow |
| Scope | skill has a clear trigger and bounded responsibility |
| Evidence | links to report/evidence/session summary |
| Non-overlap | not already covered by an existing skill |
| Size | under configured max chars, with support files if needed |
| Safety | no secrets, no unreviewed policy change |
| Provenance | created_by/provenance/created_at recorded |

## Skill Patch Policy

Patch before create.

Patch candidates:

- add one discovered caveat;
- update command preference;
- add a failure recovery path;
- clarify when the skill should not be used;
- move detailed examples into support files.

Avoid patching when:

- the evidence is single-use and weak;
- the patch would turn the skill into a transcript;
- the patch conflicts with user-authored instructions;
- the target skill is package-provided and not forked;
- the skill is pinned.

## Provenance And Curation

Recommended provenance values:

| `created_by` | `provenance` | Meaning | Automated mutation |
|---|---|---|---|
| `harness` | `package` | shipped by harness package | no |
| `user` | `foreground` | explicitly authored by user | no |
| `agent` | `foreground` | active agent edited during task | manual-review |
| `agent` | `reflection` | post-turn self-authored | yes, if not pinned |
| `agent` | `curator` | maintenance-authored | yes, if not pinned |
| `agent` | `dreaming` | synthesized from evidence | proposal first |
| `external` | `imported` | imported from another package/repo | no |

Auto-curation eligibility:

```text
created_by == "agent"
AND provenance in {"reflection", "curator", "dreaming"}
AND pinned != true
AND state in {"candidate", "quarantined", "active", "stale"}
AND target not protected
```

## Quarantine And Lineage

New agent-authored skills should not immediately become first-class durable behavior unless the host/user explicitly requested that. Reflection and dreaming outputs start as candidates or quarantined skills:

```yaml
state: candidate|quarantined|active|stale|archived
lineage:
  created_from:
    - reports/reflection/2026-05-08.md
    - memory/cold/evidence/...
  replaces: []
  absorbed_from: []
  absorbed_into: null
  promoted_by: null
```

Recommended lifecycle:

```text
candidate proposal
  -> quarantine if auto-written
  -> active after human approval, repeated use, or eval pass
  -> stale when usage drops or superseded
  -> archived after curator report + backup
```

Quarantine rules:

- quarantined skills are discoverable only when explicitly included by recall/skill index;
- they can be evaluated and patched, but should not silently influence all future tasks;
- promotion to `active` requires usage evidence, human approval, or configured eval pass;
- curator may consolidate quarantined skills aggressively because they are self-authored.

Lineage prevents skill explosion from becoming untraceable. A consolidated umbrella skill should record which candidates it absorbed, and absorbed candidates should point back to the umbrella skill.

## Report Shape

Skill production report should answer:

```yaml
report:
  type: skill-production
  path: foreground|reflection|curator|dreaming
  mode: proposal|apply
  target: skills/example/SKILL.md
  action: create|patch|archive|consolidate
  risk: low|medium|high
  evidence:
    - reports/reflection/...
    - memory/cold/evidence/...
  why_skill_not_memory: string
  existing_skill_search:
    searched: true
    candidates: []
  validation:
    schema: pass
    allowlist: pass
    protected_target: false
  rollback:
    backup: backups/...
```

## Human Review Rules

Require human approval for:

- changes to `GUIDELINE.md`, `INSTALL.md`, `harness.yaml`;
- hook behavior changes;
- install map changes;
- evaluation policy;
- permissions and safety instructions;
- user-created or imported artifacts;
- any skill that encodes external factual claims without source evidence.

## Acceptance Criteria

The skill-production system is healthy when:

1. most new knowledge becomes patches, not new skills;
2. one-off task details stay out of skills;
3. every skill has a clear trigger;
4. self-authored skills can be curated later;
5. user-authored/package/imported skills are protected;
6. every automated change has a report and provenance;
7. the same design works with hooks, background review agents, runner jobs, or manual invocation.
