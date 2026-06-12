# Capability Spec v2

> Revises `capability-spec-v1.md` under the R1 no-forward-compat channel (P2, 2026-06-12).
> harness/ has no external capability authors yet; v2 makes the breaking changes P2 needs and
> carries a version number. Independent review: the three P2 face texts (this, loop-package-v2,
> sync-abi-v2) get a dedicated adversarial doc-text review at P2 close (PD9), after their last
> amendment — that review is this revision's freeze condition.
>
> **What changed from v1** (everything else in v1 still holds; read it for the decode contract,
> validator/render catalogs, and the external-loader value/identifier vetting):
> 1. The type grammar is a CLOSED TABLE, with a reserved system-derived form.
> 2. A declared kind's kernel-required fields DERIVE from the spec (no separate hand-written line).
> 3. The `resource_kind ∈ KindCatalog` compile check becomes a reservation/namespace check against
>    the assembly-time declared kind set (the L2 gate moves from a compiled catalog to the
>    assembled one). See `loop-package-v2.md` and the PD2 declared-kind mechanism.

The DATA form of a capability: `<name>/capability.json` (external) or
`assets/capabilities/<name>.json` (first-party), compiled by `capability.FromSpec` against the
CLOSED validator and render catalogs. A spec only ever SELECTS compiled members and COMPOSES
closed validators — it never defines behavior (define≠select); everything unknown fails closed.

## Type grammar (CLOSED TABLE, ENFORCED)

`name` is the event-family segment ≡ the resource kind (for external packages, directory ≡ name ≡
kind too — the package directory IS the event family by construction). It must match
`^[a-z][a-z0-9_]*$`.

The platform's event types are a closed table of forms over the family segment. FromSpec
instantiates each form with the spec's OWN family and compares for EQUALITY — the family is bound
to the kind, never an open parameter (a well-formed-but-mismatched-prefix type is rejected, not
just free text).

| form | role | declarable in a spec? |
|---|---|---|
| `<kind>.write_candidate.observed` | `observed_type` — the host's write candidate | yes |
| `<kind>.write.proposed` | `proposed_type` — the rule's proposal (reconciler consumes only `*.proposed`) | yes |
| `<kind>.remote_commit.observed` | sync-import observation the platform mints | **no — system-derived** |

A spec that declares a system-derived form is rejected by name ("system-derived, not
spec-declarable"). New event families are added as a table ROW, not by reshaping the compile path
— this is the G7 extension point that lets P3's coordination/model-event families exist without
the grammar fighting them. The `remote_commit` form is the sync-import wire (`sync-abi-v2.md` §6);
its rule and producer land in P2 PD6.

## Declared kind + required fields

In v1 a new kind cost exactly two hand-written Go lines: a `contract.KindCatalog` entry and its
`kernel.DefaultSchemaGuard` required-fields line, kept in lockstep by a test. v2 keeps the closed
GOVERNANCE kinds (`lease`/`budget`/`receipt`/`coordination`) compiled, but user kinds enter through
the **assembly-time declared set**: the resolved capability catalog contributes its kinds, and a
kind's kernel-required fields DERIVE from the spec rather than a parallel hand-written line —

> **Required-derivation rule.** A kind's kernel-required fields are exactly the resource-header
> keys the spec's `render` produces on every write: the `static` map keys plus `content` when a
> content render member is present. Because the capability emits its full header on every propose,
> these are precisely the fields every write of the kind carries. (memory: render content →
> `{content}`; skill: render static `{"name":"project"}` → `{name}` — matching the v1 hand-written
> `DefaultSchemaGuard` lines exactly.) A spec that declares no render produces no required header
> fields. The lockstep test becomes: governance kinds stay bidirectionally pinned in code; user
> kinds have a single source — the assembled catalog.

The mechanism (splitting `KindCatalog` into compiled governance kinds + an assembled declared set,
and threading the resulting `SchemaGuard` through both the live kernel and replay so a log produced
under one kind set replays deterministically) lands in PD2. This document fixes the contract; the
wiring is in the runtime.

## Reserved namespace (G8)

A declared kind may NOT: be a governance kind (`lease`/`budget`/`receipt`/`coordination`); use the
`mnemon.` prefix; collide with a first-party event family whose diagnostics share a domain
(`sync`, `session`, `remote`); or shadow any already-loaded capability on the four axes (name,
observed type, proposed type, resource kind). External package text remains untrusted input —
values scanned by the secret/prompt-injection scanners, identifiers pattern-locked — exactly as in
v1's external-loader section.

## Unchanged from v1

The decode contract, the validator catalog, the render catalog (concat-only; no member evaluates
user content as a template), the FromSpec fail-closed checks, and the embedded-vs-external loading
differences (panic vs error path, four-axis anti-shadowing) are as `capability-spec-v1.md` states.
The Sync-import descriptor block a spec declares to opt a kind into replication is specified with
its consumer in `sync-abi-v2.md` (PD6), not here.
