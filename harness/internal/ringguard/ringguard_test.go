package ringguard

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const modulePrefix = "github.com/mnemon-dev/mnemon/"

// ring returns the ring number for a harness package path stated relative to the
// module root (e.g. "harness/internal/lifecycle/daemon"). ok is false for paths
// that are not analyzed harness packages (e.g. this guard package itself).
//
// The numbering mirrors docs/harness/16-ring-architecture.md §3. Any new harness
// package that is not classified here makes the guard fail (see DR-R-0003), which
// forces a deliberate ring assignment rather than silent drift.
func ring(rel string) (int, bool) {
	switch {
	case rel == "harness/core" || strings.HasPrefix(rel, "harness/core/"):
		// Kernel engine — the innermost tier (coarse Ring 1, docs/harness/16-ring-architecture
		// §2). It shares the numeric floor (0) with the internal trunk so a host-lifecycle
		// package importing the engine reads as INWARD (legal — the post-P2 channel wiring).
		// The one direction that must never happen — core importing harness/internal or
		// harness/cmd — is asserted directly by TestCoreEngineIsolation.
		return 0, true
	case rel == "harness/cmd/mnemon-harness":
		return 7, true // surface
	case rel == "harness/internal/ui" || strings.HasPrefix(rel, "harness/internal/ui/"):
		return 7, true // surface: the TUI cognition console (peer to cmd; imports only the facade)
	case rel == "harness/internal/app" || strings.HasPrefix(rel, "harness/internal/app/"):
		return 6, true // facade
	case rel == "harness/internal/eval",
		rel == "harness/internal/supervisor":
		return 5, true // capabilities (eval; pluggable advisory coordination supervisor)
	case rel == "harness/internal/lifecycle/daemon",
		strings.HasPrefix(rel, "harness/internal/lifecycle/daemon/"),
		rel == "harness/internal/lifecycle/reactor":
		return 4, true // orchestrator
	case rel == "harness/internal/lifecycle/runner",
		strings.HasPrefix(rel, "harness/internal/lifecycle/runner/"),
		rel == "harness/internal/hostsurface":
		return 3, true // execution / host-io
	case rel == "harness/internal/lifecycle/goal",
		rel == "harness/internal/lifecycle/goalstore",
		rel == "harness/internal/lifecycle/profile",
		rel == "harness/internal/lifecycle/proposal",
		rel == "harness/internal/lifecycle/proposalstore":
		return 2, true // stores (domain state)
	case rel == "harness/internal/lifecycle/eventlog",
		rel == "harness/internal/lifecycle/status",
		rel == "harness/internal/lifecycle/coordination",
		rel == "harness/internal/lifecycle/auditstore":
		return 1, true // substrate: event log + materialized status/coordination + audit/lineage records
	case rel == "harness/internal/lifecycle/schema",
		rel == "harness/internal/lifecycle/layout",
		rel == "harness/internal/lifecycle/corebridge",
		rel == "harness/internal/declaration":
		return 0, true // trunk / contracts (corebridge: the schema.Event <-> contract.Event seam to the kernel)
	}
	return -1, false
}

// surfaceDebt: cmd files that still import an inner package directly instead of
// going through the facade. EMPTY as of Phase R2 completion: every cmd file now
// imports only harness/internal/app. Re-add an entry only as a temporary,
// phase-tagged record if a new surface puncture is introduced and scheduled for
// removal; the steady state is empty.
var surfaceDebt = map[string]bool{}

// storeCouplingDebt: ring-2 domain stores that still import another ring-2 store.
// Empty as of Phase R3: the only entry (goalstore->auditstore) was resolved by
// reclassifying auditstore as ring-1 audit/lineage substrate (see storePackages),
// which makes that edge inward rather than sideways. Key is "importer -> imported".
var storeCouplingDebt = map[string]bool{}

// storePackages are the ring-2 domain-state stores that must stay mutually
// independent (§9 store independence): cross-store composition belongs in the
// facade. Their pure domain-type siblings (goal, proposal) are contracts a store
// may freely import. auditstore is NOT here: it is the ring-1 audit/lineage
// substrate (peer to eventlog) that domain stores legitimately write governed-
// action lineage to, so goalstore->auditstore is an inward dependency, not
// sideways coupling. Same-ring imports in other rings (status->eventlog,
// daemon->reactor, daemon->daemon/job) are legitimate intra-ring structure.
var storePackages = map[string]bool{
	"harness/internal/lifecycle/goalstore":     true,
	"harness/internal/lifecycle/profile":       true,
	"harness/internal/lifecycle/proposalstore": true,
}

func TestRingDependencyLaw(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	harnessRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // .../harness
	moduleRoot := filepath.Dir(harnessRoot)

	fset := token.NewFileSet()
	var outward, surface, storeCoupling, unclassified []string
	usedSurfaceDebt := map[string]bool{}
	usedStoreDebt := map[string]bool{}

	walkErr := filepath.WalkDir(harnessRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(moduleRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		from := filepath.ToSlash(rel)
		fromRing, known := ring(from)
		if !known {
			return nil // not an analyzed package (e.g. ringguard itself)
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil // skip unparsable file
		}
		for _, spec := range f.Imports {
			imp := strings.Trim(spec.Path.Value, `"`)
			if !strings.HasPrefix(imp, modulePrefix) {
				continue
			}
			to := strings.TrimPrefix(imp, modulePrefix)
			if !strings.HasPrefix(to, "harness/") {
				continue
			}
			toRing, knownTo := ring(to)
			if !knownTo {
				unclassified = append(unclassified, fmt.Sprintf("%s -> %s", from, to))
				continue
			}
			edge := from + " -> " + to

			// Surface rule: a ring-7 surface may import the facade (ring 6) and
			// compose sibling surface packages (ring 7) — cmd launches the ui
			// surface; the ui surface composes its own read/bind subpackages.
			// Reaching past the facade into the engine/core (rings 0-5) is still a
			// puncture, which is the property this rule protects.
			if fromRing == 7 {
				if toRing == 6 || toRing == 7 {
					continue
				}
				if surfaceDebt[to] {
					usedSurfaceDebt[to] = true
					continue
				}
				surface = append(surface, edge)
				continue
			}

			// Inward-only law: never import a higher ring.
			if toRing > fromRing {
				outward = append(outward, edge)
				continue
			}

			// Store independence: the ring-2 store packages must not import each
			// other (cross-store composition belongs in the facade).
			if storePackages[from] && storePackages[to] && from != to {
				if storeCouplingDebt[edge] {
					usedStoreDebt[edge] = true
					continue
				}
				storeCoupling = append(storeCoupling, edge)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk harness tree: %v", walkErr)
	}

	report := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		sort.Strings(items)
		t.Errorf("%s (%d):\n  %s", title, len(items), strings.Join(items, "\n  "))
	}
	report("OUTWARD import (inner ring imports outer ring)", outward)
	report("SURFACE puncture (cmd imports non-facade internal pkg, not in R2 debt)", surface)
	report("STORE coupling (ring-2 store imports another store, not in R3 debt)", storeCoupling)
	report("UNCLASSIFIED harness package (assign it a ring in ring())", unclassified)

	// Keep the debt ledgers honest: a stale entry means the dependency is gone
	// and the allowlist line should be deleted. Warn (do not fail) so mid-refactor
	// commits stay green; the entries get cleaned at phase boundaries.
	for k := range surfaceDebt {
		if !usedSurfaceDebt[k] {
			t.Logf("stale surfaceDebt entry (dependency gone, delete it): %s", k)
		}
	}
	for k := range storeCouplingDebt {
		if !usedStoreDebt[k] {
			t.Logf("stale storeCouplingDebt entry (dependency gone, delete it): %s", k)
		}
	}
}

// TestCoreEngineIsolation asserts the kernel engine is the innermost tier: harness/core
// imports NOTHING from harness/internal/** or harness/cmd/** (§2 import law — the engine
// never reaches outward into the host-lifecycle layer; that layer feeds it INWARD through
// the channel). The host -> core direction is legal and grows in P2.
func TestCoreEngineIsolation(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	harnessRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // .../harness
	moduleRoot := filepath.Dir(harnessRoot)
	coreRoot := filepath.Join(harnessRoot, "core")

	fset := token.NewFileSet()
	var offending []string
	walkErr := filepath.WalkDir(coreRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil
		}
		rel, _ := filepath.Rel(moduleRoot, path)
		for _, spec := range f.Imports {
			to := strings.TrimPrefix(strings.Trim(spec.Path.Value, `"`), modulePrefix)
			if strings.HasPrefix(to, "harness/internal/") || strings.HasPrefix(to, "harness/cmd/") {
				offending = append(offending, filepath.ToSlash(rel)+" -> "+to)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk core tree: %v", walkErr)
	}
	if len(offending) > 0 {
		sort.Strings(offending)
		t.Errorf("kernel engine must not import the host-lifecycle layer (core ↛ harness/internal|cmd):\n  %s", strings.Join(offending, "\n  "))
	}
}

// TestReleaseDoesNotImportHarness asserts the RELEASE product (module root: ./, cmd/,
// internal/ — everything OUTSIDE harness/) imports nothing under harness/ (decoupling D5,
// "zero imports either way"). The harness is an additive experiment; the shipping CLI must
// never depend on it.
func TestReleaseDoesNotImportHarness(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	harnessImport := modulePrefix + "harness/"

	fset := token.NewFileSet()
	var offending []string
	walkErr := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == moduleRoot {
				return nil
			}
			base := d.Name()
			// Skip the harness subtree (this guard is RELEASE -> harness) and every dot-dir
			// (.git, .claude worktrees, .testdata, .insight, ... are not RELEASE Go sources).
			if (base == "harness" && filepath.Dir(path) == moduleRoot) || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil
		}
		rel, _ := filepath.Rel(moduleRoot, path)
		for _, spec := range f.Imports {
			imp := strings.Trim(spec.Path.Value, `"`)
			if strings.HasPrefix(imp, harnessImport) {
				offending = append(offending, filepath.ToSlash(rel)+" -> "+strings.TrimPrefix(imp, modulePrefix))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk module root: %v", walkErr)
	}
	if len(offending) > 0 {
		sort.Strings(offending)
		t.Errorf("RELEASE must not import the harness (RELEASE ↛ harness, D5):\n  %s", strings.Join(offending, "\n  "))
	}
}
