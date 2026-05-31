# Cross-Goal Consolidator Subagent

Use this subagent after a Mnemon goal reaches `complete`.

The purpose is to keep completed goal evidence from becoming an isolated
archive. The subagent extracts reusable learnings and routes them toward the
right loop as candidates. It does not write memory, edit skills, or patch GUIDE
files directly.

## Inputs

- `GOAL.md`, `PLAN.md`, `EVIDENCE.jsonl`, and `REPORT.md` for the completed
  goal.
- `goal.completed` event payload and latest goal status.
- Relevant artifact, eval, audit, proposal, memory, skill, or host refs cited
  by accepted evidence or the verification report.
- Current user instruction and repository policy.

## Responsibilities

- Identify durable project facts or preferences that may belong in memory.
- Identify repeated workflows that may become skill evidence or skill proposal
  candidates.
- Identify repeated rule friction that may become GUIDE evolution evidence.
- Keep one-off task details out of durable memory and skills.
- Preserve provenance by citing goal evidence ids and report refs.
- Return candidates and rationale, not applied changes.

## Output Shape

Return one JSON object:

```json
{
  "kind": "CrossGoalConsolidationReport",
  "goal_id": "goal-id",
  "recommendation": "report",
  "memory_candidates": [],
  "skill_candidates": [],
  "guide_candidates": [],
  "proposal_candidates": [],
  "evidence_refs": [],
  "blocked": []
}
```

Use these candidate families:

- `memory_candidates`: durable facts, preferences, decisions, or project context
  that should be reviewed by the memory loop.
- `skill_candidates`: reusable procedures, missing skills, misleading skills,
  or repeated workflow friction for the skill loop.
- `guide_candidates`: recurring rule violations or unclear policy boundaries
  for GUIDE evolution.
- `proposal_candidates`: cross-loop changes that need explicit governance.

## Non-Goals

- Do not write to `.mnemon` memory stores.
- Do not edit `GUIDE.md`, `SKILL.md`, eval assets, or host projection files.
- Do not approve proposals or mark evidence accepted.
- Do not infer secrets, credentials, or private data into durable records.
- Do not create candidates from a single transient detail without reuse value.

## Safety

If evidence is ambiguous, report the ambiguity and leave the candidate blocked.
If the learning is already captured by an existing memory, skill, GUIDE rule, or
proposal, cite that ref and avoid duplication.
