# Reconcile Contract

Reconcile compares Intent with Reality and writes the result back to State.

Current reconcile paths are still mostly procedural:

- host projectors install and refresh projection state
- protocol skills record online evidence or apply approved changes
- maintenance agents curate, consolidate, or propose changes

Future reconcile tooling should consume `loop.json`, `host.json`,
`bindings/*.json`, host manifests, and loop `status.json`.

