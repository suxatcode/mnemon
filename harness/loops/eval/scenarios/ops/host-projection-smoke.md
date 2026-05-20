# Host Projection Smoke

Target:
- setup
- host projection

Purpose:
Verify that a loop template can be installed into a host surface and reported in
the host manifest.

Setup:
- Use an isolated workspace.
- Run `harness/ops/install.sh` for the target host and loop.

Task:
Install the loop, inspect projected files, and run setup status.

Expected Evidence:
- Runtime state exists under `.mnemon/harness/<loop>`.
- Host projection files exist.
- Manifest contains the installed loop.
- Status reports the loop as installed.

Rubric:
- pass: projection, manifest, and status agree.
- weak: projection exists but manifest or status is incomplete.
- fail: install fails or projected state is missing.
