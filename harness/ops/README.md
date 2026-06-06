# Mnemon Harness Ops

This directory contains the shared ops entrypoints for projecting canonical
Mnemon harness loops into host runtimes.

```text
harness/ops/
├── install.sh
├── status.sh
└── uninstall.sh
```

Use the shared entrypoints only for the supported memory and skill loops:

```bash
bash harness/ops/install.sh --host claude-code --loop memory
bash harness/ops/status.sh --host claude-code
bash harness/ops/uninstall.sh --host claude-code --loop memory
bash harness/ops/install.sh --host codex --loop memory
bash harness/ops/install.sh --host codex --loop skill
```

Host-specific projection logic lives under `harness/hosts/<host>/`. Loop assets
live under `harness/loops/<loop>/`.
