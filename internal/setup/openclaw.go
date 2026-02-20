package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/internal/setup/assets"
)

const openclawSteps = 2

// OpenClawInstall deploys mnemon plugin into the given OpenClaw config dir.
// configDir is typically ".openclaw" (project-local) or "~/.openclaw" (global).
// Steps: plugin files, config registration.
func OpenClawInstall(configDir string) []error {
	extensionDir := filepath.Join(configDir, "extensions", "mnemon")
	var errs []error

	fmt.Printf("\nSetting up OpenClaw (%s)...\n", configDir)

	// Step 1: Write plugin files
	if err := os.MkdirAll(extensionDir, 0755); err != nil {
		StatusError(1, openclawSteps, "Plugin", err)
		errs = append(errs, err)
	} else {
		files := map[string][]byte{
			"index.ts":             assets.OpenClawPlugin,
			"openclaw.plugin.json": assets.OpenClawManifest,
			"package.json":         assets.OpenClawPackageJSON,
		}
		allOK := true
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(extensionDir, name), content, 0644); err != nil {
				allOK = false
				errs = append(errs, err)
			}
		}
		if allOK {
			StatusOK(1, openclawSteps, "Plugin", extensionDir+" (3 files)")
		} else {
			StatusError(1, openclawSteps, "Plugin", fmt.Errorf("some files failed to write"))
		}
	}

	// Step 2: Register plugin in openclaw.json
	configPath := filepath.Join(configDir, "openclaw.json")
	if registerOpenClawConfig(configPath, extensionDir) {
		StatusOK(2, openclawSteps, "Config", configPath+" registered")
	} else {
		StatusSkipped(2, openclawSteps, "Config", configPath+" (manual registration may be needed)")
	}

	return errs
}

// registerOpenClawConfig attempts to add mnemon to openclaw.json.
// Returns false if the file uses JSON5 or other non-standard format
// that can't be safely round-tripped.
func registerOpenClawConfig(configPath, extensionDir string) bool {
	// Check if file exists and is JSON5 (has comments)
	raw, err := os.ReadFile(configPath)
	if err == nil && hasJSON5Comments(string(raw)) {
		// Can't safely roundtrip JSON5 — skip to avoid destroying comments
		return false
	}

	data, err := ReadJSONFile(configPath)
	if err != nil {
		return false
	}
	AddOpenClawPlugin(data, extensionDir)
	return WriteJSONFile(configPath, data) == nil
}

// hasJSON5Comments checks if a string contains // line comments outside of quoted strings.
func hasJSON5Comments(s string) bool {
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '/' && i+1 < len(s) && s[i+1] == '/' {
			return true
		}
	}
	return false
}

// OpenClawEject removes mnemon plugin from the given OpenClaw config dir.
func OpenClawEject(configDir string) []error {
	var errs []error

	fmt.Printf("\nRemoving OpenClaw integration (%s)...\n", configDir)

	// Step 1: Remove plugin directory
	extensionDir := filepath.Join(configDir, "extensions", "mnemon")
	if err := os.RemoveAll(extensionDir); err != nil {
		StatusError(1, 2, "Plugin", err)
		errs = append(errs, err)
	} else {
		StatusOK(1, 2, "Plugin", extensionDir+" removed")
	}
	removeIfEmpty(filepath.Join(configDir, "extensions"))

	// Step 2: Clean openclaw.json (only if standard JSON)
	configPath := filepath.Join(configDir, "openclaw.json")
	raw, _ := os.ReadFile(configPath)
	if len(raw) > 0 && hasJSON5Comments(string(raw)) {
		// JSON5 — can't safely modify, skip
		StatusSkipped(2, 2, "Config", configPath+" (JSON5, manual cleanup may be needed)")
	} else {
		data, err := ReadJSONFile(configPath)
		if err != nil {
			StatusSkipped(2, 2, "Config", configPath+" (not found)")
		} else {
			RemoveOpenClawPlugin(data)
			if err := WriteOrRemoveJSONFile(configPath, data); err != nil {
				StatusError(2, 2, "Config", err)
				errs = append(errs, err)
			} else {
				StatusOK(2, 2, "Config", configPath+" cleaned")
			}
		}
	}

	// Clean up configDir itself if empty
	removeIfEmpty(configDir)

	return errs
}

// displayPath replaces home directory with ~ for cleaner display.
func displayPath(path string) string {
	return strings.Replace(path, HomeDir(), "~", 1)
}
