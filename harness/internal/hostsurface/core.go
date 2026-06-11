package hostsurface

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
	"github.com/mnemon-dev/mnemon/harness/internal/manifest"
)

// corePaths is the host config dir + the project-local mnemon state dir.
type corePaths struct {
	configDir string
	mnemonDir string
}

// projectorCore is host-io logic shared by each backend (codex, claude-code,
// ...): path resolution, file writes, manifest paths, and common helpers. It is
// composition, not a frozen host adapter interface; each concrete projector adds
// only its host-specific surfaces.
type projectorCore struct {
	host        string // "codex" | "claude-code"
	projectRoot string
	paths       corePaths
	// shared host options (identical across hosts; set by each option parser)
	skillsDirOverride string // --host-skills-dir
	purgeMemory       bool   // --purge-memory
	purgeLibrary      bool   // --purge-library
	dryRun            bool   // --dry-run: report would-write/would-preserve, write nothing
	stdout            io.Writer
	stderr            io.Writer
	managed           *managedState // no-clobber projection state for managed definition files
}

// pathJoin is the package's display-path primitive: forward-slash joins for the host
// surface (.codex/.claude) regardless of OS, so projected refs read identically on
// every platform. It lives with projectorCore (the host-io core) rather than a
// backend file because every backend joins paths through it.
func pathJoin(base string, elems ...string) string {
	parts := append([]string{base}, elems...)
	return path.Join(parts...)
}

func (c projectorCore) resolve(displayPath string) string {
	if filepath.IsAbs(displayPath) {
		return filepath.Clean(displayPath)
	}
	return filepath.Join(c.projectRoot, filepath.FromSlash(displayPath))
}

func (c projectorCore) exists(displayPath string) bool {
	_, err := os.Stat(c.resolve(displayPath))
	return err == nil
}

// copyFile reads src from the embedded asset FS (a forward-slash key like "loops/<loop>/GUIDE.md")
// and writes it to the on-disk host surface at dstDisplay.
func (c projectorCore) copyFile(src, dstDisplay string, mode os.FileMode) error {
	data, err := fs.ReadFile(assets.FS, src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	return c.writeFile(dstDisplay, data, mode)
}

func (c projectorCore) copyFileIfMissing(src, dstDisplay string, mode os.FileMode) error {
	if _, err := os.Stat(c.resolve(dstDisplay)); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", dstDisplay, err)
	}
	return c.copyFile(src, dstDisplay, mode)
}

func (c projectorCore) writeFile(dstDisplay string, data []byte, mode os.FileMode) error {
	if c.dryRun {
		c.printf("would write %s\n", dstDisplay)
		return nil
	}
	dst := c.resolve(dstDisplay)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", dstDisplay, err)
	}
	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", dstDisplay, err)
	}
	return nil
}

func (c projectorCore) writeJSON(dstDisplay string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", dstDisplay, err)
	}
	data = append(data, '\n')
	return c.writeFile(dstDisplay, data, mode)
}

func (c projectorCore) printf(format string, args ...any) {
	fmt.Fprintf(c.stdout, format, args...)
}

func (c projectorCore) stateDir(loopName string) string {
	return pathJoin(c.paths.mnemonDir, "harness", loopName)
}

func (c projectorCore) hostManifestPath() string {
	return pathJoin(c.paths.mnemonDir, "hosts", c.host, "manifest.json")
}

// loopAsset returns the embedded-FS key (forward slashes) for a loop's projected asset.
func (c projectorCore) loopAsset(loop manifest.LoopManifest, rel string) string {
	return path.Join("loops", loop.Name, rel)
}

func (c projectorCore) readExportValue(displayPath, key string) (string, bool) {
	data, err := os.ReadFile(c.resolve(displayPath))
	if err != nil {
		return "", false
	}
	prefix := "export " + key + "="
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimPrefix(line, prefix)
		value = strings.Trim(value, `"`)
		return value, true
	}
	return "", false
}

func (c projectorCore) removeCommonStateFiles(stateDir string) error {
	for _, name := range []string{"GUIDE.md", "env.sh", "loop.json", "status.json"} {
		if err := os.Remove(c.resolve(pathJoin(stateDir, name))); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", name, err)
		}
	}
	_ = os.Remove(c.resolve(stateDir))
	return nil
}

func (c projectorCore) removeHostManifestLoop(loopName string) error {
	manifestPath := c.resolve(c.hostManifestPath())
	data, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read host manifest %s: %w", c.hostManifestPath(), err)
	}
	var manifest hostProjectionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse host manifest %s: %w", c.hostManifestPath(), err)
	}
	delete(manifest.Loops, loopName)
	if len(manifest.Loops) == 0 {
		if err := os.Remove(manifestPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove host manifest: %w", err)
		}
		return nil
	}
	manifest.UpdatedAt = nowUTC()
	return c.writeJSON(c.hostManifestPath(), manifest, 0o644)
}

// hostHookExists answers from the loop's declared intents. The error path collapses to false by
// signature; that only affects uninstall bookkeeping (Ownership.Files) — an invalid intents file
// cannot reach an installed workspace, because loop validate and projectHooks both fail closed on
// it first.
func (c projectorCore) hostHookExists(loopName, phase string) bool {
	timings, err := DeclaredHookTimings(loopName)
	if err != nil {
		return false
	}
	for _, t := range timings {
		if t == phase {
			return true
		}
	}
	return false
}

func skillID(skillPath string) string {
	dir := path.Dir(skillPath)
	if dir == "." || dir == "/" {
		return strings.TrimSuffix(path.Base(skillPath), path.Ext(skillPath))
	}
	return path.Base(dir)
}

func agentFile(loopName, subagentPath string) string {
	base := strings.TrimSuffix(path.Base(subagentPath), path.Ext(subagentPath))
	switch loopName + "." + base {
	case "skill.curator":
		return "mnemon-skill-curator.md"
	default:
		return "mnemon-" + base + ".md"
	}
}

// ---- methods shared verbatim by every host projector (hoisted from the per-host
// adapters; the displayJoin/pathJoin split was cosmetic — displayJoin called pathJoin) ----

func (p projectorCore) copyCommonCanonicalAssets(loop manifest.LoopManifest) error {
	for _, asset := range []struct {
		rel  string
		name string
		mode os.FileMode
	}{
		{rel: loop.Assets.Guide, name: "GUIDE.md", mode: 0o644},
		{rel: loop.Assets.Env, name: "env.sh", mode: 0o755},
		{rel: "loop.json", name: "loop.json", mode: 0o644},
	} {
		if err := p.copyFile(p.loopAsset(loop, asset.rel), pathJoin(p.stateDir(loop.Name), asset.name), asset.mode); err != nil {
			return err
		}
	}
	return nil
}

func (p projectorCore) prepareLoopState(loop manifest.LoopManifest) error {
	switch loop.Name {
	case "memory":
		for _, runtimeFile := range loop.Assets.RuntimeFiles {
			if err := p.copyFileIfMissing(p.loopAsset(loop, runtimeFile), pathJoin(p.stateDir(loop.Name), runtimeFile), 0o644); err != nil {
				return err
			}
		}
	case "skill":
		for _, dir := range []string{"skills/active", "skills/stale", "skills/archived", "proposals", "reports"} {
			if p.dryRun {
				continue
			}
			if err := os.MkdirAll(p.resolve(pathJoin(p.stateDir(loop.Name), dir)), 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", dir, err)
			}
		}
	}
	return nil
}

func (p projectorCore) hostSkillsDir(loopName string) string {
	if p.skillsDirOverride != "" && loopName != "memory" {
		return filepath.ToSlash(p.skillsDirOverride)
	}
	return pathJoin(p.paths.configDir, "skills")
}

func (p projectorCore) installedHostSkillsDir(loopName string, binding manifest.BindingManifest) string {
	envPath := pathJoin(binding.RuntimeSurface, "env.sh")
	envVar := "MNEMON_" + strings.ToUpper(strings.ReplaceAll(loopName, "-", "_")) + "_LOOP_HOST_SKILLS_DIR"
	if value, ok := p.readExportValue(envPath, envVar); ok {
		return value
	}
	return p.hostSkillsDir(loopName)
}

func (p projectorCore) ensureStore(ctx context.Context, storeName string) error {
	if p.dryRun {
		p.printf("would ensure mnemon store %q\n", storeName)
		return nil
	}
	mnemon, err := exec.LookPath("mnemon")
	if err != nil {
		return fmt.Errorf("mnemon binary not found in PATH; build or install it before setting a %s memory store", p.host)
	}
	list := exec.CommandContext(ctx, mnemon, "store", "list")
	list.Dir = p.projectRoot
	list.Stderr = p.stderr
	output, err := list.Output()
	if err != nil {
		return fmt.Errorf("mnemon store list: %w", err)
	}
	if !storeListContains(output, storeName) {
		create := exec.CommandContext(ctx, mnemon, "store", "create", storeName)
		create.Dir = p.projectRoot
		create.Stdout = io.Discard
		create.Stderr = p.stderr
		if err := create.Run(); err != nil {
			return fmt.Errorf("mnemon store create %s: %w", storeName, err)
		}
	}
	set := exec.CommandContext(ctx, mnemon, "store", "set", storeName)
	set.Dir = p.projectRoot
	set.Stdout = io.Discard
	set.Stderr = p.stderr
	if err := set.Run(); err != nil {
		return fmt.Errorf("mnemon store set %s: %w", storeName, err)
	}
	return nil
}

// projectHooks installs the GENERATED hook shells for every timing the loop's intents declare.
// Render errors fail the install closed — a half-migrated loop must never silently install with
// zero hooks (the legacy code skipped absent asset files, which would have masked exactly that).
func (p projectorCore) projectHooks(loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	timings, err := DeclaredHookTimings(loop.Name)
	if err != nil {
		return fmt.Errorf("hook intents for %s: %w", loop.Name, err)
	}
	if len(timings) == 0 && hasHookIntents(loop.Name) {
		return fmt.Errorf("loop %s declares hook intents but renders zero hook timings: refusing to install zero hooks", loop.Name)
	}
	for _, phase := range timings {
		content, err := RenderHook(loop.Name, p.host, phase)
		if err != nil {
			return fmt.Errorf("render hook %s/%s for %s: %w", loop.Name, phase, p.host, err)
		}
		target := pathJoin(binding.ProjectionPath, "hooks", "mnemon-"+loop.Name, phase+".sh")
		if err := p.projectManagedBytes([]byte(content), target, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func (p projectorCore) removeCanonicalState(loop manifest.LoopManifest) error {
	stateDir := p.stateDir(loop.Name)
	switch loop.Name {
	case "memory":
		if p.purgeMemory {
			return os.RemoveAll(p.resolve(stateDir))
		}
		return p.removeCommonStateFiles(stateDir)
	case "skill":
		if p.purgeLibrary {
			return os.RemoveAll(p.resolve(stateDir))
		}
		if err := p.removeCommonStateFiles(stateDir); err != nil {
			return err
		}
		for _, dir := range []string{"reports", "proposals"} {
			_ = os.Remove(p.resolve(pathJoin(stateDir, dir)))
		}
		_ = os.Remove(p.resolve(stateDir))
	default:
		return p.removeCommonStateFiles(stateDir)
	}
	return nil
}

func (p projectorCore) writeLoopStatus(loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	status := map[string]any{
		"schema_version":  2,
		"loop":            loop.Name,
		"host":            p.host,
		"phase":           "projected",
		"updated_at":      nowUTC(),
		"project_root":    p.projectRoot,
		"projection_path": p.paths.configDir,
		"state_path":      p.stateDir(loop.Name),
		"surfaces":        loop.Surfaces,
	}
	return p.writeJSON(pathJoin(p.stateDir(loop.Name), "status.json"), status, 0o644)
}

func (p projectorCore) runtimeEnvContent(loop manifest.LoopManifest, binding manifest.BindingManifest) []byte {
	envName := loopEnvName(loop.Name)
	loopDirVar := loopDirVarName(loop.Name)
	stateDir := p.stateDir(loop.Name)
	lines := []string{
		"#!/usr/bin/env bash",
		exportLine(envName, pathJoin(stateDir, "env.sh")),
		exportLine(loopDirVar, stateDir),
	}
	switch loop.Name {
	case "memory":
		lines = append(lines, `export MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES="${MNEMON_MEMORY_LOOP_MAX_NON_EMPTY_LINES:-200}"`)
	case "skill":
		hostSkillsDir := p.hostSkillsDir(loop.Name)
		lines = append(lines,
			exportLine("MNEMON_SKILL_LOOP_LIBRARY_DIR", pathJoin(stateDir, "skills")),
			exportLine("MNEMON_SKILL_LOOP_ACTIVE_DIR", pathJoin(stateDir, "skills/active")),
			exportLine("MNEMON_SKILL_LOOP_STALE_DIR", pathJoin(stateDir, "skills/stale")),
			exportLine("MNEMON_SKILL_LOOP_ARCHIVED_DIR", pathJoin(stateDir, "skills/archived")),
			exportLine("MNEMON_SKILL_LOOP_USAGE_FILE", pathJoin(stateDir, "skills/.usage.jsonl")),
			exportLine("MNEMON_SKILL_LOOP_PROPOSALS_DIR", pathJoin(stateDir, "proposals")),
			exportLine("MNEMON_SKILL_LOOP_HOST_SKILLS_DIR", hostSkillsDir),
			`export MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS="${MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS:-20}"`,
			`export MNEMON_SKILL_LOOP_PROTECTED_SKILLS="${MNEMON_SKILL_LOOP_PROTECTED_SKILLS:-skill-observe,skill-curate,skill-author,skill-manage,memory-get,memory-set}"`,
		)
	}
	content := strings.Join(lines, "\n") + "\n"
	return []byte(content)
}

func (p projectorCore) writeRuntimeEnv(loop manifest.LoopManifest, binding manifest.BindingManifest) error {
	// Route through projectManaged so env.sh is hash-recorded: a pre-existing/edited one is preserved
	// on install and on uninstall, like every other managed runtime-surface file.
	return p.projectManagedBytes(p.runtimeEnvContent(loop, binding), pathJoin(binding.RuntimeSurface, "env.sh"), 0o755)
}

// removeGeneratedSkillViews removes the host skill-view dirs the skill prime generated (marked by
// .mnemon-skill-generated), leaving any user-authored host skill untouched. It is host-agnostic (both
// hosts' skill primes write the same marker), so it lives on projectorCore.
func (c projectorCore) removeGeneratedSkillViews(hostSkillsDir string) error {
	entries, err := os.ReadDir(c.resolve(hostSkillsDir))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read host skills dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := pathJoin(hostSkillsDir, entry.Name())
		marker := pathJoin(skillDir, ".mnemon-skill-generated")
		if _, err := os.Stat(c.resolve(marker)); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("stat generated skill marker: %w", err)
		}
		if err := os.RemoveAll(c.resolve(skillDir)); err != nil {
			return fmt.Errorf("remove generated skill view: %w", err)
		}
	}
	return nil
}

func storeListContains(output []byte, storeName string) bool {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimLeft(line, "* ")
		if strings.TrimSpace(line) == storeName {
			return true
		}
	}
	return false
}
