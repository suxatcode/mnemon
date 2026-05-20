# State Contract

State is durable loop-owned data under `.mnemon/harness/<loop>/`. Source files
under `harness/loops/` are templates, not runtime state.

Every installed loop should write:

- `loop.json`
- `GUIDE.md`
- `env.sh`
- `status.json`
- loop-specific runtime files such as `MEMORY.md`, `skills/`, `reports/`, or
  eval artifacts

