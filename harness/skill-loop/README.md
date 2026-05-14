# Mnemon Skill Loop Harness Compatibility Path

The canonical Skill Loop module now lives at:

```text
harness/modules/skill-loop/
```

This directory is kept so existing install commands continue to work:

```bash
bash harness/skill-loop/setup/claude-code/install.sh
bash harness/skill-loop/setup/claude-code/uninstall.sh
```

New setup entrypoint:

```bash
bash harness/setup/install.sh --host claude-code --module skill-loop
bash harness/setup/uninstall.sh --host claude-code --module skill-loop
```
