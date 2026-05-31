# Mnemon Harness Loops

This directory contains canonical, host-agnostic loop templates.

```text
harness/loops/
├── memory/
├── skill/
├── eval/
├── goal/
└── deploy/      # extension worked example; not bound by default
```

Each loop follows the Loop Standard and declares its assets in
`loop.json`. Host-specific projection logic belongs under `harness/hosts/`.
The core first-party runtime loops are memory, skill, eval, and goal. Extra
directories may be used as extension fixtures when they validate without Go
core changes or default bindings.
