# Harness Control Plane

This directory contains the shared contracts and daemon policy for Mnemon's
experimental harness control plane. It is intentionally small: loops define
reusable lifecycle capabilities, hosts define capability surfaces, bindings
define how a loop lands on a host, and ops executes those bindings.

```text
State -> Intent -> Projection -> Reality -> Reconcile -> State
```

The source tree keeps templates, contracts, and control-plane policy here.
Runtime state is still written under `.mnemon/harness/<loop>/`.

`daemon.yaml` is the daemon-wide budget policy. New loop/plugin combinations
should not be modeled as daemon jobs; declare the loop capability in
`harness/loops/<loop>/loop.json`, bind it under `harness/bindings/`, and let the
daemon enqueue loop controllers from those declarations. Hand-written daemon
jobs are only an escape hatch and live under optional `harness/control/jobs/*.yaml`.

## Contracts

| Contract | Meaning |
| --- | --- |
| State | Canonical durable loop state under `.mnemon`. |
| Intent | Policy and desired visibility declared by loops and bindings. |
| Projection | Host-readable files, env, hooks, skills, and config. |
| Observation | Host behavior, evidence, drift, reports, and eval output. |
| Reconcile | The action set that decides whether to update state, propose work, or no-op. |
