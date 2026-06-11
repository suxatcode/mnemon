# Host Mechanics v1 (frozen) — the host-adapter face

What `hosts/<host>/host.json` may declare about HOW hooks materialize on a host. Strictly the
mechanics half of the intent/mechanics split: a host never defines behavior, it selects from
compiled members (define≠select). Adding a host = one host.json + the registration renderer;
loop packages need zero changes.

## Mechanics section (strict-decoded, closed enums)

```json
"mechanics": {
  "stdin_read":  { "default": "strict|tolerant|grep-direct", "overrides": {"<loop>": {"<timing>": "..."}} },
  "dialect":     { "default": "plain|system-message-only|codex-continue|claude-decision", "overrides": { ... } },
  "json_escape": true,
  "marker_overrides": { "<loop>": { "<timing>": false } },
  "wording_overrides": { "<loop>": { "<timing>": { "<slot>": "host wording" } } }
}
```

- **stdin_read**: how a hook consumes host stdin — `strict` (`cat`), `tolerant` (`cat || true`),
  `grep-direct` (`cat | grep -q`, no capture). Behavior-meaningful; deliberately NOT unified
  across hosts.
- **dialect**: the response envelope per (loop, timing) — `plain` (echo), `system-message-only`
  (`{"systemMessage"}`), `codex-continue` (`{"continue","stopReason","systemMessage"}`),
  `claude-decision` (`{"decision","reason"}`). Field-name sets and escaping are COMPILED members;
  the JSON shape is not authorable.
- **json_escape**: JSON dialects route interpolation through the compiled `json_escape` shell
  function. `false` is REJECTED at validation (the bare-interpolation injection face is closed
  and stays closed; the historical record lives in git, not in the schema).
- **marker_overrides**: a host may drop a marker gate an intent declares (per loop/timing).
  Validated strictly and currently unused — the last consumer was claude skill/prime, removed by
  the recorded dedup-marker unification; kept in v1 because marker applicability is genuinely
  host mechanics.
- **wording_overrides**: the ONLY free text a host owns. Overrides that nothing consumes are
  render errors (misconfiguration is loud); slots reject shell-active characters.

## Registration

Both known hosts register hooks with the identical JSON shape
`{hooks:{Event:[{hooks:[{type:"command",command:<path>}]}]}}` (codex `hooks.json`, claude
`settings.json`); generated hook files land at `<projection>/hooks/mnemon-<loop>/<timing>.sh`
through the managed no-clobber pipeline (I10), including the known-legacy-hash adoption table
that upgrades pre-ownership workspaces holding our exact retired bytes.

## Validation chain

`loop validate` renders every (host, loop, declared timing) — a fragment missing, an unsupported
mechanics combination, or an unconsumed override fails there, before any install. At install,
projectHooks fails closed on the first render error: a half-migrated loop can never silently
install with zero hooks.
