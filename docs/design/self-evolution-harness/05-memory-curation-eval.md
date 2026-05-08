# 05. Memory、Curation 与 Eval

## Memory Layers

### Hot

Directly model-facing.

```text
memory/hot/
  MEMORY.md
  USER.md
  project.md
```

Rules:

- Short.
- High confidence.
- Current task relevant.
- Declarative.
- Budgeted.
- Current user request wins.
- Exceeding budget creates demotion proposals instead of silent truncation.

### Warm

Curated middle layer.

```text
memory/warm/
  topics/
  sessions/
  projects/
  candidates/
```

Rules:

- Human-reviewable.
- Can be recalled and summarized.
- Stores session capsules, topic capsules, promotion candidates.
- Not automatically injected in full.
- Can grow larger than hot memory, but must stay searchable and summarized.

### Cold

Capacity layer.

```text
memory/cold/
  evidence/
  transcripts/
  imports/
  archive/
  index/
```

Rules:

- Large.
- Provenance-heavy.
- Searchable.
- Not directly injected.
- Used by recall and dreaming.
- May be backed by filesystem, SQLite/FTS, vector index, or other implementation details as long as Markdown reports remain the review surface.

## Budget And Overflow Policy

The harness must assume long-running memory will exceed any single Markdown file.

| Layer | Typical budget | Overflow behavior |
|---|---:|---|
| Hot | host-specific prompt budget, usually a few KB | demote detailed entries to warm; keep short pointers |
| Warm | project-readable capsules, topic files, candidates | split by topic/session; index summaries |
| Cold | high-capacity evidence and archive | compact, index, compress, or shard |

Rules:

- hot memory is never treated as append-only history;
- warm memory can hold longer summaries, but recall must summarize before injection;
- cold memory is the durable evidence store, not a prompt input;
- deletion is replaced by archive/compaction unless user explicitly requests deletion;
- budget pressure writes reports so users can inspect what moved.

## Hot/Warm/Cold Exchange

```text
observe
  -> cold evidence
  -> warm session/topic capsule
  -> promotion proposal
  -> hot memory or skill patch

curator/dreaming
  -> detect stale or repeated items
  -> demote hot detail to warm
  -> promote stable facts to hot
  -> promote repeated workflows to skill
  -> archive superseded self-authored artifacts
```

The model consumes hot memory directly. Engineering systems manage warm/cold capacity. This is the key split: model-facing memory stays small and legible; filesystem/index-backed memory absorbs long-term growth.

## Recall Ranking And NONE Gate

Recall is allowed to return no context. This is important because irrelevant memory is worse than missing memory.

Candidate ranking fields:

| Field | Meaning |
|---|---|
| `relevance` | lexical/semantic match to current task |
| `recency` | how recently the item was used or confirmed |
| `frequency` | repeated use or repeated correction count |
| `confidence` | evidence quality and user confirmation |
| `scope_match` | user/project/repo/branch/session fit |
| `risk` | cost of injecting stale or wrong instruction |
| `budget_cost` | expected output size |

Recall decision:

```text
score = relevance + recency + frequency + confidence + scope_match
penalty = risk + budget_cost
return context only if score - penalty >= threshold
otherwise return NONE
```

`NONE` output:

```yaml
type: recall
status: none
reason: "No memory above threshold for this task."
```

Rules:

- current user request always outranks recall;
- hot memory can be considered first, but still needs relevance;
- warm/cold hits must be summarized and evidence-linked;
- raw transcript is never injected;
- stale or conflicting memory should become a warning or curator signal, not context.

## Promotion

Promotion moves information toward hot memory or skill.

Triggers:

- User repeats same correction.
- Fact is reused across tasks.
- Workflow succeeds repeatedly.
- Cold evidence matches current task with high confidence.
- Curator finds a stable pattern.

Promotion proposal:

```yaml
type: promotion
from: memory/warm/topics/build.md
to: memory/hot/project.md
risk: low
reason: "Repeatedly used and user-confirmed."
evidence:
  - memory/cold/evidence/...
patch:
  action: add
  content: "Use pnpm for this repository."
```

## Demotion

Demotion moves content away from hot memory.

Triggers:

- Hot memory exceeds budget.
- Entry is stale or superseded.
- Entry is too detailed.
- Entry is procedural and should become skill.

Demotion proposal:

```yaml
type: demotion
from: memory/hot/project.md
to: memory/warm/topics/build.md
reason: "Too detailed for hot memory."
preserve_evidence: true
```

## Curator

Curator is a maintenance skill/hook. It can be triggered manually, by host scheduler, by external cron, or by the optional maintenance runner. It is not an agent loop and must not mutate active conversations.

Modes:

| Mode | Behavior |
|---|---|
| dry-run | read artifacts, write report |
| proposal | write structured proposals |
| apply | apply allowlisted low-risk patches after backup |
| rollback | restore from snapshot |

Inputs:

- `state/usage.json`
- `state/pins.json`
- active skills
- hot/warm memory
- reports

Outputs:

- `reports/curator/<timestamp>.md`
- optional patches
- optional archive moves
- updated sidecar

Curator rules:

- Class-first skill consolidation.
- Skip pinned.
- Skip package/harness/imported/user-created by default.
- Archive over delete.
- Back up before apply.
- Rewrite references only if host supports it; otherwise report needed updates.

## Dreaming

Dreaming is L4 or late L3. It should not be MVP. It is one of the strongest reasons to allow an optional maintenance runner, because it is periodic, low-priority, evidence-heavy, and can run outside active user turns.

Stages:

| Stage | Purpose | Writes |
|---|---|---|
| Light | extract candidates from recent sessions/evidence | warm candidates |
| REM | theme consolidation and narrative report | reports/dreaming |
| Deep | score and propose promotions | promotion proposals |

Dreaming must stay grounded:

- Do not promote diary text as evidence.
- Keep raw evidence links.
- Require frequency/relevance/recency score.
- Human approval for high-risk memory or guideline changes.

## Eval Gate

Eval-driven self-evolution is for higher-risk changes:

| Target | Risk | Gate |
|---|---|---|
| skill wording | low/medium | schema + sample task eval |
| hook prompt | medium | dry-run + regression cases |
| guideline | high | human approval |
| install map | high | install dry-run tests |
| code/scripts | high | tests + review |

Eval artifacts:

```text
eval/
  constraints.yaml
  datasets/
  results/
  templates/
    pr.md
```

Constraints example:

```yaml
constraints:
  max_skill_chars: 15000
  max_prompt_growth: 0.2
  required_checks:
    - validate-skill
    - check-target-allowlist
    - report-schema
  protected_targets:
    - GUIDELINE.md
    - INSTALL.md
```

## Reports

Reports are the audit surface.

Every reflection/curator/eval action must answer:

1. What changed or would change?
2. Why?
3. Which evidence supports it?
4. What risk level?
5. Was it applied or only proposed?
6. How can it be rolled back?

Report-first behavior is what keeps self-evolution reviewable.
