# Mnemon Harness Hosts

Host adapters project canonical loop modules into a concrete runtime surface.

```text
harness/hosts/
├── claude-code/
└── codex/        # future
```

Adapters should keep host-specific behavior here. Loop modules should stay
host-agnostic under `harness/modules/<module>/`.
