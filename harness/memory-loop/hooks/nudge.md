# Nudge Hook

## Runtime Moment

Run after a substantive response, task step, or completed work unit.

## Output To HostAgent

Apply `GUIDE.md` and decide whether the session produced durable information
that should be preserved in working memory.

If a working-memory update is justified, load `skills/memory_set.md` and use it
to make a small `MEMORY.md` edit. If there is no durable preference, decision,
constraint, workflow, or continuity, leave memory unchanged.

Do not write directly to Mnemon from this hook.

## Expected Effect

The HostAgent performs selective working-memory accumulation without turning
ordinary conversation into memory.
