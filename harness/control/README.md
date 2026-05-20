# Harness Control Plane

This directory contains the shared contracts for Mnemon's experimental harness
control plane. It is intentionally small: loops define reusable lifecycle
capabilities, hosts define capability surfaces, bindings define how a loop lands
on a host, and ops executes those bindings.

```text
State -> Intent -> Projection -> Reality -> Reconcile -> State
```

The source tree keeps templates and contracts here. Runtime state is still
written under `.mnemon/harness/<loop>/`.

## Contracts

| Contract | Meaning |
| --- | --- |
| State | Canonical durable loop state under `.mnemon`. |
| Intent | Policy and desired visibility declared by loops and bindings. |
| Projection | Host-readable files, env, hooks, skills, and config. |
| Observation | Host behavior, evidence, drift, reports, and eval output. |
| Reconcile | The action set that decides whether to update state, propose work, or no-op. |

