# Capability Spec v1 (frozen)

The DATA form of a built-in capability: `assets/capabilities/<name>.json`, compiled by
`capability.FromSpec` against two CLOSED catalogs. A spec can only SELECT compiled members —
it never defines behavior (define≠select); everything unknown fails closed. A new capability's
entire Go footprint is one `contract.KindCatalog` entry plus its `kernel.DefaultSchemaGuard`
lockstep line (the deliberate L2 gate).

## Shape

```json
{
  "schema_version": 1,
  "name": "<id>",
  "observed_type": "<family>.write_candidate.observed",
  "proposed_type": "<family>.write.proposed",
  "resource_kind": "<kind in KindCatalog>",
  "items_field": "<resource list field>",
  "fields": [ { "name": "<field>", "validators": [ { "id": "<member>", "params": { } } ] } ],
  "render": { "content": { "member": "<member>", "params": { } }, "static": { "k": "v" } }
}
```

## Decode contract (frozen)

- ONLY declared fields are processed; payload keys outside the declared set NEVER enter the
  Item (no leakage into governed state).
- Per string field, in declaration order: `value = strings.TrimSpace(stringField(payload, name))`;
  validators run in declared order against the processed value, FIRST error rejects; defaults
  apply to the trimmed-empty value; the processed value is what lands in the Item — and every
  declared string field emits its key (possibly `""`).
- `list:strings` is the one exception: full `stringSliceField` semantics (`[]string` / `[]any`
  dropping non-strings / comma-separated string; trimmed, empties compacted) and the key is
  OMITTED when empty. It must be its field's only validator.
- Non-string payload values read as `""` (indistinguishable from absent — by frozen contract).
- Deny messages are protocol surface: `"<name> candidate denied: <member message>"`.

## Validator catalog (closed; pure-additive)

| member | params | deny message |
|---|---|---|
| `required` | `missing_style: empty\|missing` | `empty <field>` / `missing <field>` |
| `format:skill-id` | — | `invalid <field>` (lowercase a-z0-9 dash) |
| `enum` | `values: a\|b\|c`, `message` | `<message>` |
| `default` | `value` | — (fills trimmed-empty) |
| `default-from` | `field` (declared EARLIER) | — (fills from processed field) |
| `safety:secret` | — | `secret-like content` |
| `safety:injection` | — | `prompt-injection-shaped content` |
| `safety:unsafe` | — | `unsafe content` (combined form) |
| `list:strings` | — | — (exclusive; omits empty) |

## Render catalog (closed; CONCAT-ONLY by frozen contract)

| member | params | output |
|---|---|---|
| `memory-entry-list` | — | `content` = the memory entry-list markdown |
| `bullet-list` | `title`, `field` (declared) | `content` = title + `"- "+item[field]` lines |

`static` is a literal field map. A member that evaluates user content as a template is FORBIDDEN
vocabulary — item values are joined, never executed. Render-produced keys must not collide with
`items_field` or `updated_by`, and `static` may not produce `content` alongside a content member.

## FromSpec fail-closed checks

schema_version == 1 · non-empty core fields · resource_kind ∈ KindCatalog · no duplicate fields ·
member existence · exact param key sets (missing/unknown params rejected) · `default-from` only
backward references · `list:strings` exclusivity · render collision guards. Cross-spec (loader):
duplicate capability names / observed types / proposed types rejected.

## Loading

Embedded specs are compile-time artifacts: corruption panics at init (a build defect, gated by
`TestBuiltinsLoadFromEmbeddedSpecs` + CI before merge). External capability packages
(`.mnemon/loops/<name>/capability.json`; loop-package-v1 "External capability packages") load
through `capability.ResolveCatalog`: the SAME strict decode + FromSpec compile takes the ERROR
path, never the panic — any failure (ten fail-closed fault classes, every message naming the
package path) refuses Local Mnemon boot. Two deliberate differences from embedded loading:
(a) external spec TEXT — name, enum deny messages, render `static` values, the bullet-list
`title` — is scanned by the secret/prompt-injection scanners at load time, because embedded spec
text is reviewed code pinned by golden parity (TestSpecGoldens) while external spec text is
untrusted input; (b) the merge rejects shadowing on FOUR axes (name, observed type, proposed
type, resource kind) — an external spec can never displace or impersonate an embedded one.

## Stability promise

In-surface backward compatible: members and their messages are append-only; existing member
semantics (incl. message literals, pinned by TestSpecGoldens) never change within v1.
Aliasing (`ObservedTypeAndAliases`) remains a code-level convergence policy, not spec surface.
