# Remind Hook

## Runtime Moment

Run before planning or executing a user task.

## Output To HostAgent

Apply `GUIDE.md` and decide whether prior memory could change this task.

If memory is likely to help, load `skills/memory_get.md` and follow it to run a
focused Mnemon recall. If the task is trivial, local, or fully covered by
visible context, skip recall.

Do not recall mechanically. Do not write memory from this hook.

## Expected Effect

The HostAgent makes an explicit read-memory decision before work begins.
