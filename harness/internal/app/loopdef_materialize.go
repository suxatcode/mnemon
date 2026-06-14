package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// loopdefActivator is the well-known principal under which a booting daemon records that a
// materialized loop definition is now active (G4 activation ledger, P3e): the event is a durable
// audit marker in the log, idempotent per (loopdef name, version, digest). It lives here, with the
// loopdef machinery, not in the generic contract core — "loopdef" is application vocabulary.
const loopdefActivator = contract.ActorID("loopdef@local")

// materializeLoopdefs writes every admitted loop-definition draft in the loopdef resource to a
// managed external package under .mnemon/loops/<name>/ (the D-loop Δ2/G5 step). It is the DRIVER
// bridge's job — invoked from the app reproject callback when a loopdef accept invalidates — so the
// runtime never touches the filesystem. Materialization only WRITES to disk; it never activates: a
// materialized kind is governed only after an explicit `mnemond reload` re-assembles the catalog
// (G1/G3). The package is marked default_enabled so reload governs it without an extra --loop (M3).
func materializeLoopdefs(rt *runtime.Runtime, projectRoot string) error {
	version, fields, err := rt.Resource(contract.ResourceRef{Kind: "loopdef", ID: "project"})
	if err != nil {
		return err
	}
	if version == 0 {
		return nil
	}
	items, _ := fields["items"].([]any)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		spec, _ := item["spec"].(string)
		if spec == "" {
			continue
		}
		if err := materializeDraft(projectRoot, spec, version); err != nil {
			return err
		}
	}
	return nil
}

// materializeDraft writes one validated spec draft as a managed package. The draft was already
// admitted (so it parses and compiles); here the app only adds default_enabled and writes the
// provenance marker. G5 isolation: a target dir that exists WITHOUT a .managed marker is a
// human-placed package — never clobbered; one WITH the marker is ours to regenerate.
func materializeDraft(projectRoot, specJSON string, loopdefVersion contract.Version) error {
	var spec map[string]any
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return fmt.Errorf("materialize: parse draft: %w", err)
	}
	name, _ := spec["name"].(string)
	if name == "" {
		return fmt.Errorf("materialize: draft has no name")
	}
	target := filepath.Join(projectRoot, ".mnemon", "loops", name)
	markerPath := filepath.Join(target, ".managed")
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		if _, merr := os.Stat(markerPath); os.IsNotExist(merr) {
			return nil // a human-placed package owns this name (no marker): G5 — do not clobber
		}
	}
	spec["default_enabled"] = true // M3: the spawned kind is governed once reload re-assembles
	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, "capability.json"), out, 0o600); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(specJSON))
	marker, err := json.Marshal(map[string]any{
		"materialized_by": "loopdef",
		"version":         int64(loopdefVersion),
		"digest":          hex.EncodeToString(sum[:]),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(markerPath, marker, 0o600)
}

// emitLoopdefActivations records, ON BOOT, a durable activation event for every materialized loopdef
// package present under .mnemon/loops (the G4 ledger). It is a one-time scan at boot — never a Tick
// watch (G1) — and is idempotent: the ExternalID keys on (name, version, digest), so re-booting the
// same catalog records nothing new. The event carries no rule and writes no resource; it is an audit
// marker in the event log from which "which loopdef version was active across each reload" is
// reconstructable. Best-effort: a malformed marker is skipped, never fatal to boot.
func emitLoopdefActivations(rt *runtime.Runtime, projectRoot string) error {
	loopsDir := filepath.Join(projectRoot, ".mnemon", "loops")
	entries, err := os.ReadDir(loopsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(loopsDir, e.Name(), ".managed"))
		if err != nil {
			continue // no marker = human-placed package: nothing to activate-log
		}
		var marker map[string]any
		if json.Unmarshal(raw, &marker) != nil {
			continue
		}
		digest, _ := marker["digest"].(string)
		version := marker["version"]
		env := contract.ObservationEnvelope{
			ExternalID: fmt.Sprintf("loopdef-activated:%s:%v:%s", e.Name(), version, digest),
			Event: contract.Event{
				Type:    "loopdef.activated.observed",
				Payload: map[string]any{"name": e.Name(), "version": version, "digest": digest},
			},
		}
		if _, _, err := rt.IngestTrusted(loopdefActivator, env); err != nil {
			return fmt.Errorf("record loopdef activation for %q: %w", e.Name(), err)
		}
	}
	return nil
}
