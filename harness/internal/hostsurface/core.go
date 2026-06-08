package hostsurface

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
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
	stdout      io.Writer
	stderr      io.Writer
}

func (c projectorCore) displayJoin(base string, elems ...string) string {
	return pathJoin(base, elems...)
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
		if err := os.Remove(c.resolve(c.displayJoin(stateDir, name))); err != nil && !os.IsNotExist(err) {
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

func (c projectorCore) hostHookExists(loopName, phase string) bool {
	source := path.Join("hosts", c.host, loopName, "hooks", phase+".sh")
	_, err := fs.Stat(assets.FS, source)
	return err == nil
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
