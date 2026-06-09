package hostsurface

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

// managedState tracks the no-clobber projection of one host's managed definition files: the hashes we
// last wrote (prior, loaded from the host manifest), the hashes we write this pass (next, persisted
// back), and the user-modified / pre-existing files we preserved (conflicts).
type managedState struct {
	prior     map[string]string
	next      map[string]string
	conflicts []string
}

func newManagedState() *managedState {
	return &managedState{prior: map[string]string{}, next: map[string]string{}}
}

// beginManaged resets the per-loop managed hashes and loads the prior recorded hashes for loopName
// from the existing host manifest (absent manifest -> no prior).
func (c projectorCore) beginManaged(loopName string) {
	c.managed.prior = map[string]string{}
	c.managed.next = map[string]string{}
	data, err := os.ReadFile(c.resolve(c.hostManifestPath()))
	if err != nil {
		return
	}
	var m hostProjectionManifest
	if json.Unmarshal(data, &m) != nil {
		return
	}
	if lp, ok := m.Loops[loopName]; ok && lp.Ownership.Hashes != nil {
		c.managed.prior = lp.Ownership.Hashes
	}
}

// projectManaged projects a managed definition file from the embedded asset src to dstDisplay under
// the no-clobber policy (classifyManaged): it writes + records the hash when the file is ours to
// update, or preserves + reports when the user has edited it.
func (c projectorCore) projectManaged(src, dstDisplay string, mode os.FileMode) error {
	desired, err := fs.ReadFile(assets.FS, src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	return c.projectManagedBytes(desired, dstDisplay, mode)
}

// projectManagedBytes is projectManaged for already-rendered content (e.g. a skill body with an
// appended runtime note).
func (c projectorCore) projectManagedBytes(desired []byte, dstDisplay string, mode os.FileMode) error {
	dst := c.resolve(dstDisplay)
	if classifyManaged(dst, desired, c.managed.prior[dstDisplay]) == classConflict {
		c.managed.conflicts = append(c.managed.conflicts, dstDisplay)
		c.printf("preserved user-modified %s\n", dstDisplay)
		return nil
	}
	if err := c.writeFile(dstDisplay, desired, mode); err != nil {
		return err
	}
	c.managed.next[dstDisplay] = hashBytes(desired)
	return nil
}

// removeManagedSkill removes a projected skill (its directory) ONLY if the projected SKILL.md is still
// ours — its on-disk hash matches what we recorded in the host manifest. A pre-existing skill we never
// wrote (no recorded hash) or one the user has edited is preserved + reported, so uninstall never
// deletes a file the user owns or changed. Call beginManaged(loop) first to load the recorded hashes.
func (c projectorCore) removeManagedSkill(skillFileDisplay string) error {
	abs := c.resolve(skillFileDisplay)
	current, err := os.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	prior := c.managed.prior[skillFileDisplay]
	if prior == "" || hashBytes(current) != prior {
		c.managed.conflicts = append(c.managed.conflicts, skillFileDisplay)
		c.printf("preserved %s (not Mnemon-managed or user-modified)\n", skillFileDisplay)
		return nil
	}
	return os.RemoveAll(filepath.Dir(abs))
}

// ProjectContext is the minimal context the background driver passes to ReProject: which host + loops
// to re-project, rooted at a project. The no-clobber policy applies (a pre-existing/edited file is preserved).
type ProjectContext struct {
	Host        string
	ProjectRoot string
	Loops       []string
	HostArgs    []string
}

// Report is the outcome of a re-projection: the managed files preserved because the user edited them.
type Report struct {
	Conflicts []string
}

// ReProject re-projects the managed definition files for ctx under the no-clobber policy.
// It is the entrypoint the co-hosted background driver uses on an invalidation drain (Phase 3); refs
// names the resources whose projections may need refreshing (definition files do not depend on
// resource content, so they are always re-evaluated under the no-clobber policy).
func ReProject(ctx ProjectContext, refs []contract.ResourceRef) (Report, error) {
	_ = refs
	switch ctx.Host {
	case "codex":
		return RunCodexProjectorReport(context.Background(), CodexOptions{
			ProjectRoot: ctx.ProjectRoot, Loops: ctx.Loops, HostArgs: ctx.HostArgs,
		})
	case "claude-code":
		return RunClaudeProjectorReport(context.Background(), ClaudeOptions{
			ProjectRoot: ctx.ProjectRoot, Loops: ctx.Loops, HostArgs: ctx.HostArgs,
		})
	default:
		return Report{}, fmt.Errorf("unsupported host %q", ctx.Host)
	}
}

// managedClass is the no-clobber decision for one managed definition file.
type managedClass int

const (
	classWrite    managedClass = iota // safe to (over)write: absent, equals desired, or ours-unmodified
	classConflict                     // preserve: the user edited a managed file, or a pre-existing unknown file
)

// managedMarkerVersion stamps the ownership-hash scheme so a future projector can detect an older
// marker layout and re-adopt rather than mis-preserve.
const managedMarkerVersion = 1

// classifyManaged decides whether a managed definition file at dst may be written with desired
// content, given the hash we last recorded for it (prior, empty if none). We NEVER overwrite a file we
// did not write — on install or on refresh:
//
//   - absent on disk                               -> classWrite (nothing to clobber)
//   - on-disk content already equals desired       -> classWrite (idempotent; re-install is safe)
//   - prior recorded AND on-disk matches prior      -> classWrite (still ours; safe to update)
//   - prior recorded AND on-disk differs from prior -> classConflict (user edited a managed file)
//   - no prior AND on-disk differs from desired     -> classConflict (a pre-existing unknown file —
//     the user's own — never clobbered, not even on the first install)
func classifyManaged(dst string, desired []byte, prior string) managedClass {
	current, err := os.ReadFile(dst)
	if err != nil {
		return classWrite
	}
	currentHash := hashBytes(current)
	if currentHash == hashBytes(desired) {
		return classWrite
	}
	if prior != "" && currentHash == prior {
		return classWrite
	}
	return classConflict
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
