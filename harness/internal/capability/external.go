package capability

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

// externalRootRel is the ONE external capability root of v1: <project root>/.mnemon/loops.
// LoadExternal takes an fs.FS for testability, but by frozen v1 contract that fsys is always
// rooted here, so every loader error names the real package path under this prefix.
const externalRootRel = ".mnemon/loops"

// externalNamePattern pins the package directory name (which IS the capability name, class ⑨):
// lowercase alphanumeric + dash, letter-first — the same shape as the built-in capability ids.
// It kills case aliasing ("Goal" vs "goal") and path-meaningful names at the door.
var externalNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func externalPkgPath(name string) string { return externalRootRel + "/" + name }

// LoadExternal compiles every external capability package under fsys. Each TOP-LEVEL directory is
// one package — `<name>/capability.json` in capability-spec-v1 form, strict-decoded through the
// same decodeSpec + FromSpec machinery the embedded loader uses — loaded in lexicographic package
// order (deterministic). Non-directory top-level entries (stray files, .DS_Store) are not
// packages and are ignored.
//
// fs.ErrNotExist on the root listing is the NORMAL pre-stage-5 install: empty catalog, never an
// error — a missing .mnemon/loops must not break a single existing installation.
//
// Everything else fails closed with the package path in the message: ① bad JSON / trailing
// garbage / unknown JSON keys (decodeSpec); ② unknown spec vocabulary and ③ a resource kind
// outside contract.KindCatalog (FromSpec); ⑥ any hooks/ or skills/ presence (no host projection
// assets in v1 — deliberately WIDER than loop-package-v1's minimum obligation); ⑦ statically
// derived header keys that cannot satisfy requiredFields (the kernel SchemaGuard lockstep, at
// LOAD time); ⑧ unsafe spec TEXT (external only — see scanExternalSpecText); ⑨ directory name ≠
// spec name or off-pattern. Classes ④⑤ (shadowing/dups beyond one package) are enforced by the
// shared specRegistry here and the four-axis merge in ResolveCatalog; class ⑩ (symlinks) needs
// the real OS path — fs.FS has no lstat — and lives in ResolveCatalog's screening.
func LoadExternal(fsys fs.FS, requiredFields map[contract.ResourceKind][]string) (map[string]Capability, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Capability{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read external capability root %s: %w", externalRootRel, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := map[string]Capability{}
	reg := newSpecRegistry()
	for _, name := range names {
		cap, err := loadExternalPackage(fsys, name, requiredFields)
		if err != nil {
			return nil, err
		}
		if err := reg.claim("external package "+externalPkgPath(name), cap); err != nil {
			return nil, err
		}
		out[cap.Name] = cap
	}
	return out, nil
}

func loadExternalPackage(fsys fs.FS, name string, requiredFields map[contract.ResourceKind][]string) (Capability, error) {
	pkg := externalPkgPath(name)
	if !externalNamePattern.MatchString(name) { // class ⑨ (pattern half)
		return Capability{}, fmt.Errorf("external package %s: directory name must match %s (fail-closed)", pkg, externalNamePattern)
	}
	// Class ⑥, deliberately WIDER than loop-package-v1's minimum (fragments/include/template.json):
	// hooks/ or skills/ present AT ALL — empty or not — rejects the whole package. v1 external
	// packages carry NO host projection assets; admission-equal-rights only.
	for _, sub := range []string{"hooks", "skills"} {
		if _, err := fs.Stat(fsys, path.Join(name, sub)); err == nil {
			return Capability{}, fmt.Errorf("external package %s: %s/ is forbidden in an external package (v1 carries no host projection assets; fail-closed)", pkg, sub)
		}
	}
	raw, err := fs.ReadFile(fsys, path.Join(name, "capability.json"))
	if err != nil {
		return Capability{}, fmt.Errorf("external package %s: read capability.json: %w", pkg, err)
	}
	spec, err := decodeSpec(raw) // class ①
	if err != nil {
		return Capability{}, fmt.Errorf("external package %s: parse capability.json: %w", pkg, err)
	}
	// Class ⑨ (identity half), directory-as-declaration: the directory IS the name claim.
	if spec.Name != name {
		return Capability{}, fmt.Errorf("external package %s: directory name %q must equal spec name %q (directory-as-declaration)", pkg, name, spec.Name)
	}
	cap, err := FromSpec(spec) // classes ②③ + every FromSpec fail-closed check
	if err != nil {
		return Capability{}, fmt.Errorf("external package %s: %w", pkg, err)
	}
	if err := scanExternalSpecText(spec); err != nil { // class ⑧
		return Capability{}, fmt.Errorf("external package %s: %w", pkg, err)
	}
	if err := headerCoversRequired(spec, requiredFields); err != nil { // class ⑦
		return Capability{}, fmt.Errorf("external package %s: %w", pkg, err)
	}
	return cap, nil
}

// scanExternalSpecText runs the embedded safety scanners over every spec-authored TEXT surface of
// an EXTERNAL spec: the name, each enum validator's deny message, each render static value, and
// the bullet-list title. These strings flow into deny messages and rendered governed content;
// embedded spec text is reviewed code (pinned by the golden tests), external spec text is
// untrusted input — scanned at load time, fail-closed. External path only by design.
func scanExternalSpecText(spec CapabilitySpec) error {
	type surface struct{ where, text string }
	surfaces := []surface{{"name", spec.Name}}
	for _, f := range spec.Fields {
		for _, v := range f.Validators {
			if v.ID == "enum" {
				surfaces = append(surfaces, surface{fmt.Sprintf("field %q enum message", f.Name), v.Params["message"]})
			}
		}
	}
	keys := make([]string, 0, len(spec.Render.Static))
	for k := range spec.Render.Static {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic first-error
	for _, k := range keys {
		surfaces = append(surfaces, surface{fmt.Sprintf("render static %q", k), spec.Render.Static[k]})
	}
	if c := spec.Render.Content; c != nil && c.Member == "bullet-list" {
		surfaces = append(surfaces, surface{"render bullet-list title", c.Params["title"]})
	}
	for _, s := range surfaces {
		if containsSecretLikeContent(s.text) || containsPromptInjectionShape(s.text) {
			return fmt.Errorf("unsafe spec text in %s (secret-like or prompt-injection-shaped; external spec text is untrusted input)", s.where)
		}
	}
	return nil
}

// headerCoversRequired is the load-time kernel-schema lockstep (class ⑦): the keys a capability's
// write STATICALLY produces — render static keys, "content" when a content member is selected,
// the items field, and the stamped "updated_by" — must cover every field requiredFields demands
// for the kind. Derived from the spec alone (no payload synthesis), so a package that could only
// ever produce kernel-rejected writes fails at load, not at runtime.
func headerCoversRequired(spec CapabilitySpec, requiredFields map[contract.ResourceKind][]string) error {
	produced := map[string]bool{spec.ItemsField: true, "updated_by": true}
	for k := range spec.Render.Static {
		produced[k] = true
	}
	if spec.Render.Content != nil {
		produced["content"] = true
	}
	for _, f := range requiredFields[contract.ResourceKind(spec.ResourceKind)] {
		if !produced[f] {
			return fmt.Errorf("rendered header cannot satisfy the kernel schema: kind %q requires %q but the statically produced keys never carry it (fail-closed)", spec.ResourceKind, f)
		}
	}
	return nil
}

// ResolveCatalog builds the boot capability catalog: the embedded Builtins plus every external
// capability package under <projectRoot>/.mnemon/loops — the ONLY external root in v1. It is the
// one production entry point: symlink screening on the real path (class ⑩), LoadExternal over
// os.DirFS, then a merge with FOUR-axis shadowing rejection (name, observed type, proposed type,
// resource kind) — an external package may never shadow an embedded capability, and two externals
// may not share a kind. A missing .mnemon/loops resolves to the embedded catalog.
func ResolveCatalog(projectRoot string, requiredFields map[contract.ResourceKind][]string) (map[string]Capability, error) {
	rootDir := filepath.Join(projectRoot, filepath.FromSlash(externalRootRel))
	if err := screenExternalSymlinks(rootDir); err != nil {
		return nil, err
	}
	external, err := LoadExternal(os.DirFS(rootDir), requiredFields)
	if err != nil {
		return nil, err
	}
	return mergeExternal(Builtins, external)
}

// screenExternalSymlinks is fault class ⑩, on the REAL path because fs.FS has no lstat: a
// symlinked package directory or capability.json is rejected before any fsys is built. Without
// this, os.DirFS would silently SKIP a symlinked dir (not IsDir to ReadDir) and silently FOLLOW a
// symlinked capability.json — and silent is the one thing this loader must never be.
func screenExternalSymlinks(rootDir string) error {
	entries, err := os.ReadDir(rootDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read external capability root %s: %w", externalRootRel, err)
	}
	for _, e := range entries {
		pkg := externalPkgPath(e.Name())
		if e.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("external package %s: symlinked package directory rejected (fail-closed)", pkg)
		}
		if !e.IsDir() {
			continue
		}
		if fi, err := os.Lstat(filepath.Join(rootDir, e.Name(), "capability.json")); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("external package %s: symlinked capability.json rejected (fail-closed)", pkg)
		}
	}
	return nil
}

// mergeExternal merges the external catalog into a FRESH copy of the embedded one with four-axis
// shadowing rejection. The first three axes reuse the shared specRegistry; the resource-kind axis
// is merge-time only: external may not claim a kind an embedded capability claims, and two
// externals may not share a kind (each external package owns its event family AND its kind).
// Deterministic order (sorted names) keeps the first error stable.
func mergeExternal(embedded, external map[string]Capability) (map[string]Capability, error) {
	merged := make(map[string]Capability, len(embedded)+len(external))
	reg := newSpecRegistry()
	kinds := map[contract.ResourceKind]string{}
	for _, n := range sortedKeys(embedded) {
		c := embedded[n]
		if err := reg.claim("embedded capability "+n, c); err != nil {
			return nil, err
		}
		kinds[c.ResourceKind] = c.Name
		merged[n] = c
	}
	for _, n := range sortedKeys(external) {
		c := external[n]
		src := "external package " + externalPkgPath(n)
		if err := reg.claim(src, c); err != nil {
			return nil, err
		}
		if prev, dup := kinds[c.ResourceKind]; dup {
			return nil, fmt.Errorf("%s: resource_kind %q already claimed by capability %q (external packages may not shadow)", src, c.ResourceKind, prev)
		}
		kinds[c.ResourceKind] = c.Name
		merged[n] = c
	}
	return merged, nil
}

func sortedKeys(m map[string]Capability) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
