// Package ringguard holds the architecture guard for mnemon-harness.
//
// It has no production code. Its test (ringguard_test.go) parses the import
// edges under harness/ and enforces the ring law from
// docs/harness/16-ring-architecture.md:
//
//   - inward-only: no package imports a higher-numbered ring;
//   - surface-only-facade: cmd imports only the facade (app) among internal pkgs;
//   - store independence: ring-2 store packages do not import each other.
//
// Current known violations are listed as explicit, phase-tagged allowlists that
// shrink to zero as the rings plan (docs/plan/rings/) executes. Any NEW violation
// fails the build.
package ringguard
