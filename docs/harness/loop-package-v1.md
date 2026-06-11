# Loop Package v1 (frozen) — the loop-author face

What a loop package may carry and what each part means. Together with capability-spec-v1 this is
the complete authoring surface for a loop; host-mechanics-v1 is the separate face host-adapter
authors consume. Loop packages are 100% host-neutral — nothing in a package may name a host.

## Package contents

```text
loops/<name>/
  loop.json            projection manifest (assets, surfaces, control model)   [stage-2 era]
  capability ref       assets/capabilities/<kind>.json (capability-spec-v1)
  hooks/intents.json   WHAT each lifecycle hook does (this document)
  hooks/fragments/*.sh imperative escape hatch, stitched at GENERATION time
  skills/<id>/SKILL.md judgment prose + <!-- mnemon:payload-contract --> marker
  skills/<id>/template.json  enum docs + external-id recipe feeding the generated contract
  GUIDE.md, env.sh     teaching + runtime surface assets
```

## Hook intents (closed vocabulary)

`hooks/intents.json` (schema_version 1, strict-decoded: unknown keys/members/params, trailing
data, wrong schema_version all fail closed):

- **Timings**: `prime | remind | nudge | compact` — the four lifecycle moments.
- **Gates** (per timing, ordered): `once-per-session-marker{marker}` · `two-phase-marker{marker}`
  · `if-input-field{field}` · `threshold{metric: file-non-empty-lines|usage-event-count,
  cmp: gt|ge, …env/default params}`.
- **Sections** (ordered): `env-prologue` · `local-env-control` · `control-env` · `banner{lines}` ·
  `control-call{actions}` · `file-emit{var,path,header}` · `include{fragment}`; control actions:
  `observe{event_type, external_id_prefix, payload}` · `status` · `pull-mirror{…}`.
- **Response** (per timing): `role: one-liner | message | block` with `text` or threshold-selected
  `over`/`under` slots.
- **Wording convention**: intents carry the canonical default text for every slot; hosts override
  via host.json `wording_overrides` (host-mechanics-v1). Slots are PROSE ONLY — the decoder rejects
  `"`, `` ` ``, `\`, newlines and `$(`; everything else is inert because every slot
  interpolation site in the compiled templates is double-quoted (that quoting context is part of
  the frozen template contract).

The vocabulary is CLOSED: a member not listed here does not exist; adding one is a versioned
change to this document plus the compiled catalog.

## Fragments (frozen restriction)

Fragments are loop-side shell bodies referenced by `include{fragment}`. They are concatenated
into the generated hook at GENERATION time and never evaluated by the generator or the runtime.
**v1: fragments are valid only in EMBEDDED loop packages.** Today this is enforced structurally —
the renderer reads fragments exclusively from the embedded asset FS; no external loader exists.
**Binding stage-5 obligation: any external-package loader MUST reject a package containing
`hooks/fragments/`, an `include` intent, or a `skills/*/template.json` whose recipe/notes were not
shipped embedded — fail closed, with a regression test, before external packages gain "same
rights" loading.** (template.json recipe/notes are LLM-facing and recipe is shell-by-design; they
carry the same trust requirement as fragments.) Relaxing any of this requires a new version of
this document.

## SKILL generation rule

`SKILL.md` keeps frontmatter + judgment prose (when to use, what to reject, confidence guidance)
and marks the payload-mechanics position with `<!-- mnemon:payload-contract -->`. At projection
the marker is replaced by a section GENERATED from the capability spec (fields, requiredness,
enum values, safety scans) plus `template.json` (external-id recipe, enum docs). Single source:
a spec field rename changes the projected SKILL or breaks the token-coverage gate — there is no
reverse dependency. Skill template-instance renaming machinery (`<loop>-set/get` for arbitrary
loops) is deliberately deferred until an external package needs it.

## Migration provenance

The generated hooks were proven byte-identical to the 16 retired handwritten shells (empty patch
table) before the legacy assets were deleted; the standing pin is the golden-hash table in
hookgen's tests. Two deliberate unifications are recorded in history: claude compact reason
escaping (closed an injection face) and claude skill prime session-dedup marker.
