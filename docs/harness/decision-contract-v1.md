# Decision Contract v1 (frozen)

The semantics and ordering rules of the governed pipeline — the contract replay, the Shadow
promotion gate, and every future evolution tool are held to. Joins wasm-abi-v0, capability-spec-v1
and the Sync DTOs as a frozen face.

## The six-step pipeline

```text
1 STAMP   (intake)      reserved suffixes (*.proposed/*.diagnostic) rejected FIRST; type format
                        validated; schema_version/id/ts/actor stamped from the AUTHENTICATED
                        principal; forgeable fields (based_on/projection_ref/ingest_seq) zeroed;
                        exactly-once by (principal, external_id), append+dedupe in one tx.
2 DISPATCH (tick)       events processed in IngestSeq order (= log rowid, the ONLY ordering key;
                        ts is provenance, never orders). Each OBSERVED event is evaluated against
                        the actor's scoped projection AT DISPATCH TIME — before this tick's
                        reconcile. Reserved-suffix events are skipped (they bypass the pre-gate).
3 REDUCE  (rule set)    deny-priority reduction over the rules that Handle the type. A rule
                        VerdictDeny mints a durable *.diagnostic and NO kernel decision; only
                        VerdictPropose continues.
4 MINT    (bridge)      the proposal becomes a TRUSTED *.proposed event: type/actor from the
                        REGISTERED rule (never the payload), based_on = the dispatched view's
                        read-set, projection provenance + correlation stamped; any decoded write
                        outside the dispatched scope is refused here.
5 APPLY   (kernel)      authority (actor x kind) → read-set staleness → CAS (BasedOn) → schema
                        guard; the decision AND its writes land in ONE transaction. Statuses:
                        accepted / rejected / deferred — all durable.
6 SIDE-EFFECTS          per accepted decision: one idempotent "invalidation" outbox row carrying
                        d.NewVersions (+ sync commit recording); per non-accept: one durable
                        *.diagnostic. Driven by the decision-sink cursor (crash-recoverable,
                        exactly-once per decision).
```

## Determinism statement (I6)

Same event log + same configuration + same rule versions ⇒ same decision sequence, with these
frozen qualifications:

- **Masked dynamic fields:** DecisionID and AppliedAt are minted per run; comparison happens
  after `maskDynamic` (which also canonicalizes Conflicts/NewVersions ordering). Everything else
  — status, reason, conflicts, new versions, ingest seq, actor, correlation — must reproduce.
- **Modes single source:** the platform's zero-config modes are `contract.DefaultModes()`
  (reject / projection_read_set / strict). The live server and replay BOTH reference it; the
  equality is pinned by test (a divergence historically let replay defer what live rejected).
- **Replay scope:** replay re-reconciles the logged *.proposed events under permissive authority,
  so it reproduces conflict (CAS/read-stale), schema and malformed rejects — kernel-AUTHZ rejects
  are excluded by design (the live authority is evidenced by the log itself; replay re-derives,
  it does not re-police). Rule-deny steps contribute zero decisions on both sides; their
  *.diagnostic events are the durable record.
- **Dispatch-time view:** a rule sees the projection as of its event's dispatch, NOT the final
  state. Two proposals minted against the same view in one tick mean the second is read-stale —
  rejected under the default modes. This is the price of replayability and is contract, not bug.

## Shadow promotion gate

`Shadow(events, subs, live, candidate)` answers "would promoting this rule set change behavior?"
by re-running BOTH rule sets over the OBSERVED events at dispatch-time state (proposals evolve the
throwaway kernel; evaluations are read-only). The comparison covers verdict, proposal (type +
payload), job, trusted origin actor, **Reasons**, and diagnostics — Reasons are not advisory: they
land verbatim in durable *.diagnostic events, so a reword IS a behavior change (pinned by the
gate test mutating one spec enum message). The report counts diffs; the operator gates promotion
on Clean. Capability-spec-level evolution (capability-spec-v1.md) is exactly what this gate
arbitrates.
