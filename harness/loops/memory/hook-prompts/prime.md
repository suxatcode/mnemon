# Prime Hook

## Runtime Moment

Run at session start, agent bootstrap, or first system prompt assembly.

## Output To HostAgent

Check Local Mnemon status, refresh the local memory mirror when reachable, then
load the current `MEMORY.md` and `GUIDE.md` into the system prompt.

`MEMORY.md` is a compact, prompt-facing mirror for this project, not the
canonical write target. `GUIDE.md` is policy: it explains when memory should be
read or written.

Do not contact a Remote Workspace during Prime. Do not load memory outside the
scoped Local Mnemon result. Use `memory-get` later only if the task appears to
need more focused prior memory.

## Expected Effect

The HostAgent starts the session with a current local memory mirror and memory
judgment rules, but without performing remote sync or memory writeback.
