# Mnemon Harness Hosts

Host adapters project canonical loop templates into a concrete runtime surface.

```text
harness/hosts/
├── claude-code/
└── codex/
```

Adapters should keep host-specific behavior here. Loop templates should stay
host-agnostic under `harness/loops/<loop>/`.

The Codex adapter projects protocol skills into repo-local `.codex/skills` and
keeps canonical loop state under `.mnemon/harness/<loop>`. This shape lets the
real Codex app-server load the projected skills from an isolated eval workspace.
