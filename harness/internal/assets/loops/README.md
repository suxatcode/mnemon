# Mnemon Harness Loops

This directory contains canonical, host-agnostic loop templates.

```text
harness/internal/assets/loops/
├── memory/
└── skill/
```

Each loop follows the Loop Standard and declares its assets in
`loop.json`. Host-specific projection logic belongs under
`harness/internal/assets/hosts/`. The loop/host/binding manifests and their
asset files are embedded into the `mnemon-harness` binary (`go:embed`), so
setup/refresh/validate read them from the binary, not from an on-disk source
tree.

## Cutover (fresh-setup-only; no migrator)

There is no migration from any legacy on-disk `.mnemon/` file tree. The local
governed store is created on **first serve** (`mnemon-harness local run`, which
opens `.mnemon/harness/local/governed.db` via the store). `mnemon-harness setup`
only writes the Agent Workspace projection plus the Mnemon Workspace config
(`config.json` with `store_path=.mnemon/harness/local/governed.db`),
`bindings.json`, `env.sh`, and the access token — it does not create or migrate
`governed.db`. Any pre-existing OLD file-tree `.mnemon/` is legacy: it is
neither read nor migrated.

The first-party product loops are memory and skill. Non-product prototype loop
assets are not kept in this runtime tree.
