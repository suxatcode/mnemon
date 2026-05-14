# Mnemon Memory Loop Harness Compatibility Path

The canonical Memory Loop module now lives at:

```text
harness/modules/memory-loop/
```

This directory is kept so existing install commands continue to work:

```bash
bash harness/memory-loop/setup/claude-code/install.sh
bash harness/memory-loop/setup/claude-code/uninstall.sh
```

New setup entrypoint:

```bash
bash harness/setup/install.sh --host claude-code --module memory-loop
bash harness/setup/uninstall.sh --host claude-code --module memory-loop
```
