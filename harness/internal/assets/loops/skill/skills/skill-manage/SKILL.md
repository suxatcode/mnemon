---
name: skill-manage
description: Submit approved skill lifecycle and content changes to Local Mnemon.
---

# skill-manage

Use this skill only after a proposal has been approved by the user or by an
explicit host policy.

## Boundary

This skill submits approved skill declarations to Local Mnemon. It does not edit
host skill directories or canonical files directly. New active skills become
host-visible after Local Mnemon accepts the declaration and the host projection
refreshes.

Use the Local Mnemon environment installed by setup when it is available:

```bash
source .mnemon/harness/local/env.sh 2>/dev/null || true
```

## Allowed MVP Operations

- submit an approved active skill declaration
- submit approved `SKILL.md` content drafted by `skill-author`
- submit a replacement declaration for an existing skill
- submit lifecycle status changes: `active`, `stale`, or `archived`
- submit metadata or usage notes needed by the lifecycle

## Procedure

1. Read the approved proposal and confirm the intended operation.
2. Check `MNEMON_SKILL_LOOP_PROTECTED_SKILLS`; do not modify protected skills
   unless the approval explicitly covers the exception.
3. Keep skill ids hyphen-case: lowercase letters, numbers, and `-`. Preserve a
   non-conforming id only when an external host compatibility boundary requires
   it.
4. Submit the smallest approved declaration through Local Mnemon:

```bash
mnemon-harness control observe \
  --addr "${MNEMON_CONTROL_ADDR:-http://127.0.0.1:8787}" \
  --principal "$MNEMON_CONTROL_PRINCIPAL" \
  ${MNEMON_CONTROL_TOKEN_FILE:+--token-file "$MNEMON_CONTROL_TOKEN_FILE"} \
  --type skill.write_candidate_observed \
  --external-id "skill-${SKILL_ID}-${STATUS}-${PROPOSAL_ID}" \
  --payload '{"skill_id":"release-checklist","name":"release-checklist","status":"active","content":"...","source":"approved-proposal","confidence":"high"}'
```

5. Prefer `status:"archived"` over deletion.
6. Do not edit the host skill surface directly. Let Local Mnemon and Prime
   regenerate mirrors.
7. Record the submitted declaration in the proposal or usage log when useful.

## Safety

If the proposal is ambiguous, risky, or conflicts with current repository state,
stop and ask for approval instead of guessing.
