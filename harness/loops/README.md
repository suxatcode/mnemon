# Mnemon Harness Loops

This directory contains canonical, host-agnostic loop templates.

```text
harness/loops/
├── memory/
└── skill/
```

Each loop follows the Loop Standard and declares its assets in
`loop.json`. Host-specific projection logic belongs under `harness/hosts/`.
The first-party product loops are memory and skill. Non-product prototype loop
assets are not kept in this runtime tree.
