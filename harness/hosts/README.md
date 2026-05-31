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

Both Codex and Claude Code adapters can project the goal loop's `mnemon-goal`
skill. The skill uses `mnemon-harness goal` commands for durable project goal
state while leaving host-owned continuation mechanisms such as Codex `/goal`
outside Mnemon's authority.
