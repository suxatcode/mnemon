# Compact Hook

## Runtime Moment

Run before context compaction, summarization, or any boundary where important
session context may be lost.

## Output To HostAgent

Apply `GUIDE.md` and decide whether any critical continuity should survive the
context boundary.

If so, load `skills/memory-set/SKILL.md` and submit only the minimal necessary
Local Mnemon memory candidate. Preserve decisions, constraints, unresolved
continuity, and state that would otherwise be lost.

Do not save the whole conversation. Do not perform mirror cleanup from this
hook.

## Expected Effect

The HostAgent preserves important continuity before compaction without
performing offline consolidation.
