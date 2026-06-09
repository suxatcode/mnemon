# WASM Rule ABI v0 (frozen on paper; NOT built)

The Rule Host's second implementation seam. A future `wasmRule` is a pure adapter implementing the
existing `rule.Rule` interface (`harness/internal/rule/rule.go`) — registered in the same select-only
trusted registry as the native rules (one code-level map entry, by design: the seam is
interface-open, not config-open). Zero kernel / runtime / bridge changes are required. Do not build
the host until a rule exists that native Go cannot ship.

## Call shape (one guest call per dispatched event)

```text
input  (host -> guest, JSON):  RuleInput
  {
    "event":      contract.Event        // server-stamped observation (id/ts/actor are TRUSTED inputs)
    "view":       projection.Projection // the actor's scoped, digested dispatch-time view
  }

output (guest -> host, JSON):  contract.RuleDecision
  {
    "verdict":  "allow" | "deny" | "warn" | "propose" | "enqueue_job" | "request_evidence",
    "reasons":  [string],
    "proposal": contract.ProposedEvent?  // {type, payload:{writes:[contract.ResourceWrite]}}
  }
```

Plus the four identity methods the registry needs, supplied by the registration entry (NOT the
guest): `ID()`, `Actor()`, `Emits()`, `Handles(eventType)`.

## The three trust rules (already enforced host-side against a hostile rule)

1. **Return-only.** A rule never writes: the kernel is the sole canonical writer; a `propose`
   verdict is only an INTENT. The guest gets no store/kernel/filesystem capability.
2. **Emit-type borrowing is rejected in the reducer** (`rule.go` reducer): a proposal whose type is
   not the rule's registered `Emits()` is refused — a guest cannot mint another capability's event.
3. **Write identity is stamped server-side.** `ProposalActor` comes from the registered `Actor()`
   (trusted field marked `json:"-"` in contract — unforgeable from payload), and the bridge
   (`runtime.Bridge.Stamp`) rejects any decoded write outside the actor's dispatched scope before a
   `*.proposed` event exists.

Sandbox/runtime choices (wazero, fuel limits, hash-pinned modules) were proven PROOF-ONLY on
`feat/full-control-plane` and stay out of tree until the five WASM preconditions in
`.plan/harness-local-wasm-evolution.md` hold.
