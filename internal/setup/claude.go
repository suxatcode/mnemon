package setup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/internal/setup/assets"
)

const claudeSteps = 4

// ClaudeInstall deploys mnemon integration into the given Claude Code config dir.
// configDir is typically ".claude" (project-local) or "~/.claude" (global).
// Steps: skill, hook (recall), hook (remind), settings.
func ClaudeInstall(configDir string) []error {
	hooksDir := filepath.Join(configDir, "hooks", "mnemon")
	var errs []error

	fmt.Printf("\nSetting up Claude Code (%s)...\n", configDir)

	// Step 1: Write skill
	skillDir := filepath.Join(configDir, "skills", "mnemon")
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		StatusError(1, claudeSteps, "Skill", err)
		errs = append(errs, err)
	} else if err := os.WriteFile(skillPath, assets.ClaudeSkill, 0644); err != nil {
		StatusError(1, claudeSteps, "Skill", err)
		errs = append(errs, err)
	} else {
		StatusOK(1, claudeSteps, "Skill", skillPath)
	}

	// Step 2: Write recall hook
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		StatusError(2, claudeSteps, "Hook: recall", err)
		errs = append(errs, err)
	} else {
		recallPath := filepath.Join(hooksDir, "user_prompt.sh")
		if err := os.WriteFile(recallPath, assets.ClaudeUserPromptHook, 0755); err != nil {
			StatusError(2, claudeSteps, "Hook: recall", err)
			errs = append(errs, err)
		} else {
			StatusOK(2, claudeSteps, "Hook: recall", recallPath)
		}
	}

	// Step 3: Write remind hook
	remindPath := filepath.Join(hooksDir, "stop.sh")
	if err := os.WriteFile(remindPath, assets.ClaudeStopHook, 0755); err != nil {
		StatusError(3, claudeSteps, "Hook: remind", err)
		errs = append(errs, err)
	} else {
		StatusOK(3, claudeSteps, "Hook: remind", remindPath)
	}

	// Step 4: Register hooks in settings.json
	settingsPath := filepath.Join(configDir, "settings.json")
	data, err := ReadJSONFile(settingsPath)
	if err != nil {
		StatusError(4, claudeSteps, "Settings", err)
		errs = append(errs, err)
	} else {
		AddClaudeHooks(data, hooksDir)
		if err := WriteJSONFile(settingsPath, data); err != nil {
			StatusError(4, claudeSteps, "Settings", err)
			errs = append(errs, err)
		} else {
			StatusUpdated(4, claudeSteps, "Settings", settingsPath)
		}
	}

	return errs
}

// ClaudeEject removes mnemon integration from the given Claude Code config dir.
func ClaudeEject(configDir string) []error {
	var errs []error

	fmt.Printf("\nRemoving Claude Code integration (%s)...\n", configDir)

	// Step 1: Remove hooks directory
	hooksDir := filepath.Join(configDir, "hooks", "mnemon")
	if err := os.RemoveAll(hooksDir); err != nil {
		StatusError(1, 3, "Hooks", err)
		errs = append(errs, err)
	} else {
		StatusOK(1, 3, "Hooks", hooksDir+" removed")
	}
	removeIfEmpty(filepath.Join(configDir, "hooks"))

	// Step 2: Clean settings.json
	settingsPath := filepath.Join(configDir, "settings.json")
	data, err := ReadJSONFile(settingsPath)
	if err != nil {
		StatusError(2, 3, "Settings", err)
		errs = append(errs, err)
	} else {
		RemoveClaudeHooks(data)
		if err := WriteOrRemoveJSONFile(settingsPath, data); err != nil {
			StatusError(2, 3, "Settings", err)
			errs = append(errs, err)
		} else {
			StatusOK(2, 3, "Settings", settingsPath+" cleaned")
		}
	}

	// Step 3: Remove skill directory
	skillDir := filepath.Join(configDir, "skills", "mnemon")
	if err := os.RemoveAll(skillDir); err != nil {
		StatusError(3, 3, "Skill", err)
		errs = append(errs, err)
	} else {
		StatusOK(3, 3, "Skill", skillDir+" removed")
	}
	removeIfEmpty(filepath.Join(configDir, "skills"))

	// Clean up configDir itself if empty
	removeIfEmpty(configDir)

	return errs
}
