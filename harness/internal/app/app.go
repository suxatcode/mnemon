// Package app is the harness facade (ring 6 in docs/harness/16-ring-architecture).
//
// It exposes one application-level operation per surface need and is the only
// package allowed to span the engine rings (stores, orchestrator, capabilities).
// Surfaces — the cmd CLI today, a read-mostly gui later — depend on app and the
// standard library only; they never import the inner lifecycle packages directly.
// app defines its own input/result types so that adding or moving a surface never
// reaches past this ring.
//
// Cross-ring composition lives here too: when an operation needs two inner
// packages (e.g. complete a goal in the store and append a completion event to
// the event log), app composes them. Inner packages must not reach sideways to do
// it.
package app

import (
	"encoding/json"
	"fmt"
	"io"
)

// Harness is the facade handle. It carries the project root and constructs inner
// stores per operation, mirroring the original per-command behavior.
type Harness struct {
	root string
}

// New returns a facade bound to the given project root ("." for the cwd).
func New(root string) *Harness {
	if root == "" {
		root = "."
	}
	return &Harness{root: root}
}

// writeJSON prints value as indented JSON followed by a newline. It mirrors the
// CLI's --json output exactly, marshaling the inner types so JSON output stays
// byte-identical after a surface migration.
func writeJSON(out io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(out, string(data))
	return nil
}
