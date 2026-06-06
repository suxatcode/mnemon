# State Contract

State is the durable canonical record of loop-owned data.

**Canonical symbol:** governed state is mutated ONLY through the kernel — the single
WRITE authority. A governed change becomes a `contract.ResourceVersion` (per-resource
`Version`, `+1` per accepted write) in `kernel.Store` via the rule pre-gate + CAS writer
(D1); no path writes governed state without the kernel admitting it first. In this
transitional phase the durable loop files under `.mnemon/harness/<loop>/` remain the
host-side **read-authority mirror**, materialized by `internal/hostsurface` only AFTER
the kernel accepts (P2.1 shim, option a); relocating the read model onto the kernel
log (so the file becomes a pure derived projection) is the deferred final step. Source
files under `harness/loops/` are templates, not runtime state.

Every installed loop's host mirror should carry:

- `loop.json`
- `GUIDE.md`
- `env.sh`
- `status.json`
- loop-specific runtime files such as `MEMORY.md`, `skills/`, `reports/`, or
  eval artifacts
