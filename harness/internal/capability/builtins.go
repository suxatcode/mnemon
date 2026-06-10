package capability

import (
	"fmt"
	"io/fs"
	"path"
	"sort"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
)

// Builtins is the trusted registry, built by compiling the EMBEDDED capability specs
// (assets/capabilities/*.json) against the closed catalogs. Embedded specs are compile-time
// artifacts: a corrupt one is a build defect, caught by TestBuiltinsLoadFromEmbeddedSpecs and the
// gates before merge — hence the panic at package init, not an error path. (External spec
// directories are stage 5 and will take loadBuiltins' error path, never the panic.)
var Builtins = mustLoadBuiltins()

func mustLoadBuiltins() map[string]Capability {
	b, err := loadBuiltins(assets.FS)
	if err != nil {
		panic(fmt.Sprintf("embedded capability specs are a build artifact and must compile: %v", err))
	}
	return b
}

// loadBuiltins parses every capabilities/*.json under fsys and compiles it via FromSpec
// (fail-closed). Cross-spec uniqueness is enforced: duplicate names, observed types, or proposed
// types are rejected — two capabilities must never claim the same event family.
func loadBuiltins(fsys fs.FS) (map[string]Capability, error) {
	entries, err := fs.ReadDir(fsys, "capabilities")
	if err != nil {
		return nil, fmt.Errorf("read capabilities dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && path.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := map[string]Capability{}
	seenObserved, seenProposed := map[string]string{}, map[string]string{}
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, path.Join("capabilities", name))
		if err != nil {
			return nil, fmt.Errorf("read capability spec %s: %w", name, err)
		}
		spec, err := decodeSpec(raw)
		if err != nil {
			return nil, fmt.Errorf("parse capability spec %s: %w", name, err)
		}
		cap, err := FromSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("compile capability spec %s: %w", name, err)
		}
		if _, dup := out[cap.Name]; dup {
			return nil, fmt.Errorf("capability spec %s: duplicate capability name %q", name, cap.Name)
		}
		if prev, dup := seenObserved[cap.ObservedType]; dup {
			return nil, fmt.Errorf("capability spec %s: observed type %q already claimed by %q", name, cap.ObservedType, prev)
		}
		if prev, dup := seenProposed[cap.ProposedType]; dup {
			return nil, fmt.Errorf("capability spec %s: proposed type %q already claimed by %q", name, cap.ProposedType, prev)
		}
		seenObserved[cap.ObservedType], seenProposed[cap.ProposedType] = cap.Name, cap.Name
		out[cap.Name] = cap
	}
	return out, nil
}
