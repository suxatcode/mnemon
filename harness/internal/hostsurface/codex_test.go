package hostsurface

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/profile"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// coordFixture builds a coordination event (host "" -> unscoped host, as an
// apply-emitted topology event is).
func coordFixture(id, typ, host string, payload map[string]any) schema.Event {
	loop := "coordination"
	ev := schema.Event{
		SchemaVersion: schema.Version,
		ID:            id,
		TS:            "2026-05-30T10:00:00Z",
		Type:          typ,
		Loop:          &loop,
		Actor:         "host-agent",
		Source:        "test",
		CorrelationID: "c",
		Payload:       payload,
	}
	if host != "" {
		h := host
		ev.Host = &h
	}
	return ev
}

func seedCoordinationLedger(t *testing.T, projectRoot string) {
	t.Helper()
	store, err := eventlog.New(projectRoot)
	if err != nil {
		t.Fatalf("eventlog.New: %v", err)
	}
	for _, ev := range []schema.Event{
		coordFixture("k1", coordination.EventTaskClaimed, "codex", map[string]any{coordination.FieldTaskID: "T1"}),
		coordFixture("k2", coordination.EventTaskClaimed, "claude-code", map[string]any{coordination.FieldTaskID: "T2"}),
		// An applied merge: T2 joined into T1 (no host — emitted by mnemon on apply).
		coordFixture("k3", coordination.EventTaskJoined, "", map[string]any{coordination.FieldTaskID: "T2", coordination.FieldJoinedInto: "T1"}),
	} {
		if err := store.Append(ev); err != nil {
			t.Fatalf("append %s: %v", ev.ID, err)
		}
	}
}

func readCoordinationFragment(t *testing.T, path string) coordination.View {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("coordination fragment not projected: %v", err)
	}
	var v coordination.View
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("parse coordination fragment: %v", err)
	}
	return v
}

// TestRunCodexProjectorPullsCoordinationFragment proves Band 4's projection: a
// host pulls its own claims via COORDINATION.json on install.
func TestRunCodexProjectorPullsCoordinationFragment(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writePlanFixture(t, root)
	seedCoordinationLedger(t, projectRoot)

	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
		Stdout:          &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("install: %v", err)
	}
	frag := readCoordinationFragment(t, filepath.Join(projectRoot, ".codex", "mnemon-memory", "COORDINATION.json"))
	if len(frag.Tasks) != 1 || frag.Tasks[0].ID != "T1" || frag.Tasks[0].Owner != "codex" {
		t.Fatalf("codex coordination fragment should hold its own task T1, got %#v", frag.Tasks)
	}
}

// seedProfileEntry records one durable profile entry targeted at (host, loop),
// the canonical source the projector pulls from when projecting a fragment.
func seedProfileEntry(t *testing.T, projectRoot, entryID string, now time.Time, host, loop string) {
	t.Helper()
	store, err := profile.New(projectRoot)
	if err != nil {
		t.Fatalf("profile.New: %v", err)
	}
	if _, _, err := store.AddEntry(profile.AddEntryOptions{
		EntryID:           entryID,
		Type:              "preference",
		Summary:           entryID,
		Content:           "content for " + entryID,
		Evidence:          []profile.EvidenceRef{{Type: "manual", Ref: "test-evidence"}},
		ProjectionTargets: []profile.ProjectionTarget{{Host: host, Loop: loop}},
		Now:               now,
	}); err != nil {
		t.Fatalf("AddEntry %s: %v", entryID, err)
	}
}

func readProfileFragment(t *testing.T, path string) profile.Profile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile fragment %s: %v", path, err)
	}
	var frag profile.Profile
	if err := json.Unmarshal(data, &frag); err != nil {
		t.Fatalf("parse profile fragment: %v", err)
	}
	return frag
}

// TestRunCodexProjectorPullsScopedProfileFragment proves the pull side of the
// memory loop: an applied profile entry targeted at codex/memory is projected to
// the Codex runtime surface as PROFILE.json, scoped (an entry for another host is
// excluded). This is the loop the Band 0 gate requires: an applied route=memory
// entry changes what the next run pulls.
func TestRunCodexProjectorPullsScopedProfileFragment(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writePlanFixture(t, root)

	seedProfileEntry(t, projectRoot, "codex-pref", time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC), "codex", "memory")
	seedProfileEntry(t, projectRoot, "claude-pref", time.Date(2026, 5, 30, 0, 0, 1, 0, time.UTC), "claude-code", "memory")

	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
		Stdout:          &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("RunCodexProjector install returned error: %v", err)
	}

	frag := readProfileFragment(t, filepath.Join(projectRoot, ".codex", "mnemon-memory", "PROFILE.json"))
	if len(frag.Entries) != 1 {
		t.Fatalf("codex fragment should hold only the codex/memory entry, got %d: %#v", len(frag.Entries), frag.Entries)
	}
	if frag.Entries[0].ID != "codex-pref" {
		t.Fatalf("codex fragment entry = %q, want codex-pref", frag.Entries[0].ID)
	}
}

func TestRunCodexProjectorInstallsStatusAndUninstallsMemory(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writePlanFixture(t, root)
	configDir := filepath.Join(projectRoot, ".codex")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	configToml := "[hooks]\n# user inline hooks stay owned by Codex/user config\n"
	configTomlPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configTomlPath, []byte(configToml), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	userHooks := `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/usr/bin/true",
            "statusMessage": "user-owned mnemon-memory marker is not ownership"
          }
        ]
      }
    ]
  }
}
`
	hooksPath := filepath.Join(configDir, "hooks.json")
	if err := os.WriteFile(hooksPath, []byte(userHooks), 0o644); err != nil {
		t.Fatalf("write user hooks.json: %v", err)
	}

	var installOut bytes.Buffer
	err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
		Stdout:          &installOut,
	})
	if err != nil {
		t.Fatalf("RunCodexProjector install returned error: %v", err)
	}
	for _, rel := range []string{
		".mnemon/harness/memory/GUIDE.md",
		".mnemon/harness/memory/env.sh",
		".mnemon/harness/memory/loop.json",
		".mnemon/harness/memory/MEMORY.md",
		".mnemon/harness/memory/status.json",
		".codex/mnemon-memory/env.sh",
		".codex/mnemon-memory/GUIDE.md",
		".codex/skills/memory-get/SKILL.md",
		".codex/hooks/mnemon-memory/prime.sh",
		".codex/hooks/mnemon-memory/remind.sh",
		".codex/hooks/mnemon-memory/nudge.sh",
		".codex/hooks/mnemon-memory/compact.sh",
		".codex/hooks.json",
		".mnemon/hosts/codex/manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected projected file %s: %v", rel, err)
		}
	}
	for _, rel := range []string{
		".codex/hooks/mnemon-memory/prime.sh",
		".codex/hooks/mnemon-memory/remind.sh",
		".codex/hooks/mnemon-memory/nudge.sh",
		".codex/hooks/mnemon-memory/compact.sh",
	} {
		info, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("stat projected hook %s: %v", rel, err)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("expected projected hook %s to be executable, mode %v", rel, info.Mode())
		}
	}
	skillData, err := os.ReadFile(filepath.Join(projectRoot, ".codex", "skills", "memory-get", "SKILL.md"))
	if err != nil {
		t.Fatalf("read projected skill: %v", err)
	}
	if !strings.Contains(string(skillData), "## Codex Projection") {
		t.Fatalf("projected skill missing runtime note:\n%s", string(skillData))
	}
	hooks := readJSONMap(t, hooksPath)
	for event, command := range map[string]string{
		"SessionStart":     ".codex/hooks/mnemon-memory/prime.sh",
		"UserPromptSubmit": ".codex/hooks/mnemon-memory/remind.sh",
		"Stop":             ".codex/hooks/mnemon-memory/nudge.sh",
		"PreCompact":       ".codex/hooks/mnemon-memory/compact.sh",
	} {
		if !codexHookEventHasCommand(hooks, event, command) {
			t.Fatalf("hooks.json missing %s command %s:\n%#v", event, command, hooks)
		}
	}
	if !containsString(hooks, "/usr/bin/true") {
		t.Fatalf("user hook was not preserved:\n%#v", hooks)
	}

	manifestData, err := os.ReadFile(filepath.Join(projectRoot, ".mnemon", "hosts", "codex", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest hostProjectionManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	entry, ok := manifest.Loops["memory"]
	if !ok {
		t.Fatalf("manifest missing memory entry: %#v", manifest.Loops)
	}
	if len(entry.Ownership.Files) == 0 {
		t.Fatalf("manifest missing ownership files: %#v", entry.Ownership)
	}
	for _, want := range []string{
		".codex/hooks.json",
		".codex/hooks/mnemon-memory/prime.sh",
	} {
		if !stringSliceContains(entry.Ownership.Files, want) {
			t.Fatalf("manifest ownership missing %s: %#v", want, entry.Ownership.Files)
		}
	}

	var statusOut bytes.Buffer
	err = RunCodexProjector(context.Background(), "status", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Stdout:          &statusOut,
	})
	if err != nil {
		t.Fatalf("RunCodexProjector status returned error: %v", err)
	}
	if !strings.Contains(statusOut.String(), "Codex memory:") || !strings.Contains(statusOut.String(), "loop:   installed") {
		t.Fatalf("unexpected status:\n%s", statusOut.String())
	}

	err = RunCodexProjector(context.Background(), "uninstall", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
	})
	if err != nil {
		t.Fatalf("RunCodexProjector uninstall returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".codex", "skills", "memory-get")); !os.IsNotExist(err) {
		t.Fatalf("expected projected memory skill to be removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".codex", "hooks", "mnemon-memory")); !os.IsNotExist(err) {
		t.Fatalf("expected projected memory hooks to be removed, got %v", err)
	}
	afterHooks := readJSONMap(t, hooksPath)
	for event, command := range map[string]string{
		"SessionStart":     ".codex/hooks/mnemon-memory/prime.sh",
		"UserPromptSubmit": ".codex/hooks/mnemon-memory/remind.sh",
		"Stop":             ".codex/hooks/mnemon-memory/nudge.sh",
		"PreCompact":       ".codex/hooks/mnemon-memory/compact.sh",
	} {
		if codexHookEventHasCommand(afterHooks, event, command) {
			t.Fatalf("expected mnemon hook command to be removed after uninstall: %s %s\n%#v", event, command, afterHooks)
		}
	}
	if !containsString(afterHooks, "/usr/bin/true") {
		t.Fatalf("expected user hook to remain after uninstall:\n%#v", afterHooks)
	}
	if !containsString(afterHooks, "user-owned mnemon-memory marker") {
		t.Fatalf("expected user statusMessage marker text to remain after uninstall:\n%#v", afterHooks)
	}
	afterConfigToml, err := os.ReadFile(configTomlPath)
	if err != nil {
		t.Fatalf("read config.toml after uninstall: %v", err)
	}
	if string(afterConfigToml) != configToml {
		t.Fatalf("config.toml was modified:\n%s", string(afterConfigToml))
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".mnemon", "harness", "memory", "MEMORY.md")); err != nil {
		t.Fatalf("expected MEMORY.md to be preserved, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".mnemon", "hosts", "codex", "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("expected host manifest to be removed, got %v", err)
	}
}

func TestRunCodexProjectorDiffAndDryRun(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writePlanFixture(t, root)

	var dryRunOut bytes.Buffer
	err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
		HostArgs:        []string{"--dry-run"},
		Stdout:          &dryRunOut,
	})
	if err != nil {
		t.Fatalf("RunCodexProjector dry-run returned error: %v", err)
	}
	if !strings.Contains(dryRunOut.String(), "would create .codex/skills/memory-get/SKILL.md") {
		t.Fatalf("unexpected dry-run output:\n%s", dryRunOut.String())
	}
	if !strings.Contains(dryRunOut.String(), "would create .codex/hooks/mnemon-memory/prime.sh") ||
		!strings.Contains(dryRunOut.String(), "would create .codex/hooks.json (metadata)") {
		t.Fatalf("dry-run output missing hook projection:\n%s", dryRunOut.String())
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".codex", "skills", "memory-get", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not write projected skill, got %v", err)
	}

	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
	}); err != nil {
		t.Fatalf("RunCodexProjector install returned error: %v", err)
	}
	var cleanDiff bytes.Buffer
	if err := RunCodexProjector(context.Background(), "diff", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
		Stdout:          &cleanDiff,
	}); err != nil {
		t.Fatalf("RunCodexProjector clean diff returned error: %v", err)
	}
	if !strings.Contains(cleanDiff.String(), "no changes") {
		t.Fatalf("expected clean diff, got:\n%s", cleanDiff.String())
	}

	skillPath := filepath.Join(projectRoot, ".codex", "skills", "memory-get", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("local edit\n"), 0o644); err != nil {
		t.Fatalf("edit projected skill: %v", err)
	}
	var dirtyDiff bytes.Buffer
	if err := RunCodexProjector(context.Background(), "diff", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
		Stdout:          &dirtyDiff,
	}); err != nil {
		t.Fatalf("RunCodexProjector dirty diff returned error: %v", err)
	}
	if !strings.Contains(dirtyDiff.String(), "update .codex/skills/memory-get/SKILL.md") {
		t.Fatalf("expected projected skill drift, got:\n%s", dirtyDiff.String())
	}
	items, err := CollectCodexDrift(context.Background(), CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
	})
	if err != nil {
		t.Fatalf("CollectCodexDrift returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one drift item, got %#v", items)
	}
	if items[0].Host != "codex" || items[0].Loop != "memory" || items[0].Action != "update" || items[0].Target != ".codex/skills/memory-get/SKILL.md" {
		t.Fatalf("unexpected drift item: %#v", items[0])
	}
	if items[0].Text() != "update .codex/skills/memory-get/SKILL.md" {
		t.Fatalf("unexpected drift item text: %s", items[0].Text())
	}
}

func TestRunCodexReconcileRepairsManagedHooksContentDrift(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writePlanFixture(t, root)
	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
	}); err != nil {
		t.Fatalf("RunCodexProjector install returned error: %v", err)
	}
	hooksPath := filepath.Join(projectRoot, ".codex", "hooks.json")
	hooks := readJSONMap(t, hooksPath)
	events := hooks["hooks"].(map[string]any)
	stopEntries := events["Stop"].([]any)
	managedStop := stopEntries[0].(map[string]any)
	managedStop["hooks"] = append(managedStop["hooks"].([]any), map[string]any{
		"type":    "command",
		"command": "echo dogfood-drift",
	})
	events["Stop"] = append(stopEntries, map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": "/usr/bin/true",
			},
		},
	})
	writeJSONMap(t, hooksPath, hooks)

	items, err := CollectCodexDrift(context.Background(), CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
	})
	if err != nil {
		t.Fatalf("CollectCodexDrift returned error: %v", err)
	}
	if len(items) != 1 || items[0].Target != ".codex/hooks.json" || items[0].Action != "update" {
		t.Fatalf("expected hooks.json update drift, got %#v", items)
	}

	result, err := RunCodexReconcile(context.Background(), CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
	})
	if err != nil {
		t.Fatalf("RunCodexReconcile returned error: %v", err)
	}
	if result.Status != "repaired" || len(result.Repaired) != 1 {
		t.Fatalf("expected one repaired drift item, got %#v", result)
	}
	repairedHooks := readJSONMap(t, hooksPath)
	if containsString(repairedHooks, "echo dogfood-drift") {
		t.Fatalf("managed hook drift was not removed:\n%#v", repairedHooks)
	}
	if !containsString(repairedHooks, "/usr/bin/true") {
		t.Fatalf("user-owned hook entry was not preserved:\n%#v", repairedHooks)
	}
	if !codexHookEventHasCommand(repairedHooks, "Stop", ".codex/hooks/mnemon-memory/nudge.sh") {
		t.Fatalf("managed Stop hook was not restored:\n%#v", repairedHooks)
	}
}

func TestRunCodexProjectorInstallsAndUninstallsSkillHooks(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writeSkillPlanFixture(t, root)

	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"skill"},
	}); err != nil {
		t.Fatalf("RunCodexProjector skill install returned error: %v", err)
	}
	for _, rel := range []string{
		".codex/hooks/mnemon-skill/prime.sh",
		".codex/hooks/mnemon-skill/remind.sh",
		".codex/hooks/mnemon-skill/nudge.sh",
		".codex/hooks/mnemon-skill/compact.sh",
		".codex/hooks.json",
		".codex/mnemon-skill/env.sh",
		".codex/skills/skill-observe/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected projected skill file %s: %v", rel, err)
		}
	}
	envData, err := os.ReadFile(filepath.Join(projectRoot, ".codex", "mnemon-skill", "env.sh"))
	if err != nil {
		t.Fatalf("read skill env: %v", err)
	}
	if !strings.Contains(string(envData), "MNEMON_SKILL_LOOP_REVIEW_MIN_EVENTS") {
		t.Fatalf("skill runtime env missing review threshold:\n%s", string(envData))
	}

	hooks := readJSONMap(t, filepath.Join(projectRoot, ".codex", "hooks.json"))
	for event, command := range map[string]string{
		"SessionStart": ".codex/hooks/mnemon-skill/prime.sh",
		"Stop":         ".codex/hooks/mnemon-skill/nudge.sh",
		"PreCompact":   ".codex/hooks/mnemon-skill/compact.sh",
	} {
		if !codexHookEventHasCommand(hooks, event, command) {
			t.Fatalf("hooks.json missing %s command %s:\n%#v", event, command, hooks)
		}
	}
	if codexHookEventHasCommand(hooks, "UserPromptSubmit", ".codex/hooks/mnemon-skill/remind.sh") {
		t.Fatalf("skill remind hook should not be registered by default:\n%#v", hooks)
	}

	generatedSkill := filepath.Join(projectRoot, ".codex", "skills", "generated-skill")
	if err := os.MkdirAll(generatedSkill, 0o755); err != nil {
		t.Fatalf("mkdir generated skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generatedSkill, ".mnemon-skill-generated"), nil, 0o644); err != nil {
		t.Fatalf("write generated skill marker: %v", err)
	}

	if err := RunCodexProjector(context.Background(), "uninstall", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"skill"},
	}); err != nil {
		t.Fatalf("RunCodexProjector skill uninstall returned error: %v", err)
	}
	afterHooks := readJSONMap(t, filepath.Join(projectRoot, ".codex", "hooks.json"))
	for event, command := range map[string]string{
		"SessionStart": ".codex/hooks/mnemon-skill/prime.sh",
		"Stop":         ".codex/hooks/mnemon-skill/nudge.sh",
		"PreCompact":   ".codex/hooks/mnemon-skill/compact.sh",
	} {
		if codexHookEventHasCommand(afterHooks, event, command) {
			t.Fatalf("expected skill hook command to be removed after uninstall: %s %s\n%#v", event, command, afterHooks)
		}
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".codex", "hooks", "mnemon-skill")); !os.IsNotExist(err) {
		t.Fatalf("expected projected skill hooks to be removed, got %v", err)
	}
	if _, err := os.Stat(generatedSkill); !os.IsNotExist(err) {
		t.Fatalf("expected generated skill view to be removed, got %v", err)
	}
}

func TestRunCodexProjectorInstallsAndUninstallsGoalHooks(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writeGoalPlanFixture(t, root)

	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"goal"},
	}); err != nil {
		t.Fatalf("RunCodexProjector goal install returned error: %v", err)
	}
	for _, rel := range []string{
		".codex/hooks/mnemon-goal/prime.sh",
		".codex/hooks/mnemon-goal/remind.sh",
		".codex/hooks/mnemon-goal/nudge.sh",
		".codex/hooks/mnemon-goal/compact.sh",
		".codex/hooks.json",
		".codex/mnemon-goal/env.sh",
		".codex/skills/mnemon-goal/SKILL.md",
		".mnemon/harness/goals",
		".mnemon/harness/status/goals",
	} {
		if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected projected goal file %s: %v", rel, err)
		}
	}
	hooks := readJSONMap(t, filepath.Join(projectRoot, ".codex", "hooks.json"))
	for event, command := range map[string]string{
		"SessionStart":     ".codex/hooks/mnemon-goal/prime.sh",
		"UserPromptSubmit": ".codex/hooks/mnemon-goal/remind.sh",
		"Stop":             ".codex/hooks/mnemon-goal/nudge.sh",
		"PreCompact":       ".codex/hooks/mnemon-goal/compact.sh",
	} {
		if !codexHookEventHasCommand(hooks, event, command) {
			t.Fatalf("hooks.json missing %s command %s:\n%#v", event, command, hooks)
		}
	}

	if err := RunCodexProjector(context.Background(), "uninstall", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"goal"},
	}); err != nil {
		t.Fatalf("RunCodexProjector goal uninstall returned error: %v", err)
	}
	afterHooks := readJSONMap(t, filepath.Join(projectRoot, ".codex", "hooks.json"))
	for event, command := range map[string]string{
		"SessionStart":     ".codex/hooks/mnemon-goal/prime.sh",
		"UserPromptSubmit": ".codex/hooks/mnemon-goal/remind.sh",
		"Stop":             ".codex/hooks/mnemon-goal/nudge.sh",
		"PreCompact":       ".codex/hooks/mnemon-goal/compact.sh",
	} {
		if codexHookEventHasCommand(afterHooks, event, command) {
			t.Fatalf("expected goal hook command to be removed after uninstall: %s %s\n%#v", event, command, afterHooks)
		}
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".codex", "hooks", "mnemon-goal")); !os.IsNotExist(err) {
		t.Fatalf("expected projected goal hooks to be removed, got %v", err)
	}
}

func TestRunCodexProjectorInstallsAndUninstallsEvalHooks(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writeEvalPlanFixture(t, root)

	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"eval"},
	}); err != nil {
		t.Fatalf("RunCodexProjector eval install returned error: %v", err)
	}
	for _, rel := range []string{
		".codex/hooks/mnemon-eval/prime.sh",
		".codex/hooks/mnemon-eval/remind.sh",
		".codex/hooks/mnemon-eval/nudge.sh",
		".codex/hooks/mnemon-eval/compact.sh",
		".codex/hooks.json",
		".codex/mnemon-eval/env.sh",
		".codex/skills/eval-plan/SKILL.md",
		".mnemon/harness/eval/scenarios",
		".mnemon/harness/eval/suites",
		".mnemon/harness/eval/rubrics",
	} {
		if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected projected eval file %s: %v", rel, err)
		}
	}
	hooks := readJSONMap(t, filepath.Join(projectRoot, ".codex", "hooks.json"))
	for event, command := range map[string]string{
		"SessionStart":     ".codex/hooks/mnemon-eval/prime.sh",
		"UserPromptSubmit": ".codex/hooks/mnemon-eval/remind.sh",
		"Stop":             ".codex/hooks/mnemon-eval/nudge.sh",
		"PreCompact":       ".codex/hooks/mnemon-eval/compact.sh",
	} {
		if !codexHookEventHasCommand(hooks, event, command) {
			t.Fatalf("hooks.json missing %s command %s:\n%#v", event, command, hooks)
		}
	}

	if err := RunCodexProjector(context.Background(), "uninstall", CodexOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"eval"},
	}); err != nil {
		t.Fatalf("RunCodexProjector eval uninstall returned error: %v", err)
	}
	afterHooks := readJSONMap(t, filepath.Join(projectRoot, ".codex", "hooks.json"))
	for event, command := range map[string]string{
		"SessionStart":     ".codex/hooks/mnemon-eval/prime.sh",
		"UserPromptSubmit": ".codex/hooks/mnemon-eval/remind.sh",
		"Stop":             ".codex/hooks/mnemon-eval/nudge.sh",
		"PreCompact":       ".codex/hooks/mnemon-eval/compact.sh",
	} {
		if codexHookEventHasCommand(afterHooks, event, command) {
			t.Fatalf("expected eval hook command to be removed after uninstall: %s %s\n%#v", event, command, afterHooks)
		}
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".codex", "hooks", "mnemon-eval")); !os.IsNotExist(err) {
		t.Fatalf("expected projected eval hooks to be removed, got %v", err)
	}
}

func TestParseCodexHostOptionsRejectsUnknownFlags(t *testing.T) {
	_, err := parseCodexHostOptions([]string{"--unknown"})
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return value
}

func writeJSONMap(t *testing.T, path string, value map[string]any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeSkillPlanFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "skill")
	hostDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "hook-prompts"),
		filepath.Join(loopDir, "skills", "skill-observe"),
		filepath.Join(loopDir, "skills", "skill-curate"),
		filepath.Join(loopDir, "skills", "skill-author"),
		filepath.Join(loopDir, "skills", "skill-manage"),
		filepath.Join(hostDir, "skill", "hooks"),
		hostDir,
		bindingDir,
	} {
		mkdir(t, dir)
	}
	for _, path := range []string{
		filepath.Join(loopDir, "GUIDE.md"),
		filepath.Join(loopDir, "env.sh"),
		filepath.Join(loopDir, "hook-prompts", "prime.md"),
		filepath.Join(loopDir, "hook-prompts", "remind.md"),
		filepath.Join(loopDir, "hook-prompts", "nudge.md"),
		filepath.Join(loopDir, "hook-prompts", "compact.md"),
		filepath.Join(loopDir, "skills", "skill-observe", "SKILL.md"),
		filepath.Join(loopDir, "skills", "skill-curate", "SKILL.md"),
		filepath.Join(loopDir, "skills", "skill-author", "SKILL.md"),
		filepath.Join(loopDir, "skills", "skill-manage", "SKILL.md"),
	} {
		writeFile(t, path, "fixture\n")
	}
	for _, name := range []string{"prime.sh", "remind.sh", "nudge.sh", "compact.sh"} {
		writeFile(t, filepath.Join(hostDir, "skill", "hooks", name), "#!/usr/bin/env bash\necho fixture\n")
	}
	writeFile(t, filepath.Join(loopDir, "loop.json"), `{
  "schema_version": 2,
  "name": "skill",
  "control_model": {
    "state": [],
    "intent": "fixture",
    "reality": [],
    "reconcile": []
  },
  "entity_profiles": {},
  "surfaces": {
    "projection": [],
    "observation": []
  },
  "assets": {
    "guide": "GUIDE.md",
    "env": "env.sh",
    "runtime_files": [],
    "hook_prompts": {
      "prime": "hook-prompts/prime.md",
      "remind": "hook-prompts/remind.md",
      "nudge": "hook-prompts/nudge.md",
      "compact": "hook-prompts/compact.md"
    },
    "skills": [
      "skills/skill-observe/SKILL.md",
      "skills/skill-curate/SKILL.md",
      "skills/skill-author/SKILL.md",
      "skills/skill-manage/SKILL.md"
    ],
    "subagents": []
  },
  "host_adapters": {
    "codex": "../../hosts/codex"
  }
}`)
	writeFile(t, filepath.Join(hostDir, "host.json"), `{
  "schema_version": 2,
  "name": "codex",
  "surfaces": {
    "projection": [".codex/skills", ".codex/hooks", ".codex/hooks.json", ".codex/mnemon-skill"],
    "observation": []
  },
  "lifecycle_mapping": {},
  "supports": {
    "skills": true,
    "hooks": true
  }
}`)
	writeFile(t, filepath.Join(bindingDir, "codex.skill.json"), `{
  "schema_version": 1,
  "name": "codex.skill",
  "host": "codex",
  "loop": "skill",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-skill",
  "lifecycle_mapping": {
    "prime": "SessionStart",
    "remind": "UserPromptSubmit",
    "nudge": "Stop",
    "compact": "PreCompact"
  },
  "reconcile": ["observe", "curate", "propose", "manage", "no-op"]
}`)
}

func writeGoalPlanFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "goal")
	hostDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "hook-prompts"),
		filepath.Join(loopDir, "skills", "mnemon-goal"),
		filepath.Join(hostDir, "goal", "hooks"),
		hostDir,
		bindingDir,
	} {
		mkdir(t, dir)
	}
	for _, path := range []string{
		filepath.Join(loopDir, "GUIDE.md"),
		filepath.Join(loopDir, "env.sh"),
		filepath.Join(loopDir, "hook-prompts", "prime.md"),
		filepath.Join(loopDir, "hook-prompts", "remind.md"),
		filepath.Join(loopDir, "hook-prompts", "nudge.md"),
		filepath.Join(loopDir, "hook-prompts", "compact.md"),
		filepath.Join(loopDir, "skills", "mnemon-goal", "SKILL.md"),
	} {
		writeFile(t, path, "fixture\n")
	}
	for _, name := range []string{"prime.sh", "remind.sh", "nudge.sh", "compact.sh"} {
		writeFile(t, filepath.Join(hostDir, "goal", "hooks", name), "#!/usr/bin/env bash\necho fixture\n")
	}
	writeFile(t, filepath.Join(loopDir, "loop.json"), `{
  "schema_version": 2,
  "name": "goal",
  "control_model": {
    "state": [],
    "intent": "fixture",
    "reality": [],
    "reconcile": []
  },
  "entity_profiles": {},
  "surfaces": {
    "projection": [],
    "observation": []
  },
  "assets": {
    "guide": "GUIDE.md",
    "env": "env.sh",
    "runtime_files": [],
    "hook_prompts": {
      "prime": "hook-prompts/prime.md",
      "remind": "hook-prompts/remind.md",
      "nudge": "hook-prompts/nudge.md",
      "compact": "hook-prompts/compact.md"
    },
    "skills": ["skills/mnemon-goal/SKILL.md"],
    "subagents": []
  },
  "host_adapters": {
    "codex": "../../hosts/codex"
  }
}`)
	writeFile(t, filepath.Join(hostDir, "host.json"), `{
  "schema_version": 2,
  "name": "codex",
  "surfaces": {
    "projection": [".codex/skills", ".codex/hooks", ".codex/hooks.json", ".codex/mnemon-goal"],
    "observation": []
  },
  "lifecycle_mapping": {},
  "supports": {
    "skills": true,
    "hooks": true
  }
}`)
	writeFile(t, filepath.Join(bindingDir, "codex.goal.json"), `{
  "schema_version": 1,
  "name": "codex.goal",
  "host": "codex",
  "loop": "goal",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-goal",
  "lifecycle_mapping": {
    "prime": "SessionStart",
    "remind": "UserPromptSubmit",
    "nudge": "Stop",
    "compact": "PreCompact"
  },
  "reconcile": ["init", "plan", "record_evidence", "verify", "complete", "block", "pause", "resume", "link_host", "no-op"]
}`)
}

func writeEvalPlanFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "eval")
	hostDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "hook-prompts"),
		filepath.Join(loopDir, "skills", "eval-plan"),
		filepath.Join(loopDir, "skills", "eval-run"),
		filepath.Join(loopDir, "skills", "eval-analyze"),
		filepath.Join(loopDir, "skills", "eval-improve"),
		filepath.Join(hostDir, "eval", "hooks"),
		hostDir,
		bindingDir,
	} {
		mkdir(t, dir)
	}
	for _, path := range []string{
		filepath.Join(loopDir, "GUIDE.md"),
		filepath.Join(loopDir, "env.sh"),
		filepath.Join(loopDir, "hook-prompts", "prime.md"),
		filepath.Join(loopDir, "hook-prompts", "remind.md"),
		filepath.Join(loopDir, "hook-prompts", "nudge.md"),
		filepath.Join(loopDir, "hook-prompts", "compact.md"),
		filepath.Join(loopDir, "skills", "eval-plan", "SKILL.md"),
		filepath.Join(loopDir, "skills", "eval-run", "SKILL.md"),
		filepath.Join(loopDir, "skills", "eval-analyze", "SKILL.md"),
		filepath.Join(loopDir, "skills", "eval-improve", "SKILL.md"),
	} {
		writeFile(t, path, "fixture\n")
	}
	for _, name := range []string{"prime.sh", "remind.sh", "nudge.sh", "compact.sh"} {
		writeFile(t, filepath.Join(hostDir, "eval", "hooks", name), "#!/usr/bin/env bash\necho fixture\n")
	}
	writeFile(t, filepath.Join(loopDir, "loop.json"), `{
  "schema_version": 2,
  "name": "eval",
  "control_model": {
    "state": [],
    "intent": "fixture",
    "reality": [],
    "reconcile": []
  },
  "entity_profiles": {},
  "surfaces": {
    "projection": [],
    "observation": []
  },
  "assets": {
    "guide": "GUIDE.md",
    "env": "env.sh",
    "runtime_files": [],
    "hook_prompts": {
      "prime": "hook-prompts/prime.md",
      "remind": "hook-prompts/remind.md",
      "nudge": "hook-prompts/nudge.md",
      "compact": "hook-prompts/compact.md"
    },
    "skills": [
      "skills/eval-plan/SKILL.md",
      "skills/eval-run/SKILL.md",
      "skills/eval-analyze/SKILL.md",
      "skills/eval-improve/SKILL.md"
    ],
    "subagents": []
  },
  "host_adapters": {
    "codex": "../../hosts/codex"
  }
}`)
	writeFile(t, filepath.Join(hostDir, "host.json"), `{
  "schema_version": 2,
  "name": "codex",
  "surfaces": {
    "projection": [".codex/skills", ".codex/hooks", ".codex/hooks.json", ".codex/mnemon-eval"],
    "observation": []
  },
  "lifecycle_mapping": {},
  "supports": {
    "skills": true,
    "hooks": true
  }
}`)
	writeFile(t, filepath.Join(bindingDir, "codex.eval.json"), `{
  "schema_version": 1,
  "name": "codex.eval",
  "host": "codex",
  "loop": "eval",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-eval",
  "lifecycle_mapping": {
    "prime": "SessionStart",
    "remind": "UserPromptSubmit",
    "nudge": "Stop",
    "compact": "PreCompact"
  },
  "reconcile": ["plan", "run", "analyze", "improve", "retire", "no-op"]
}`)
}

// CollectCodexDrift is a test-only helper that reports projection drift without
// applying repairs. The live drift path uses collectCodexDrift via RunCodexReconcile.
func CollectCodexDrift(ctx context.Context, opts CodexOptions) ([]DriftItem, error) {
	_ = ctx
	projector, loops, err := newCodexProjector("diff", opts)
	if err != nil {
		return nil, err
	}
	return collectCodexDrift(projector, loops)
}

// codexHookEventHasCommand is a test-only helper that reports whether a Codex
// settings document declares the given command for a hook event.
func codexHookEventHasCommand(data map[string]any, event, command string) bool {
	hooks, ok := data["hooks"].(map[string]any)
	if !ok {
		return false
	}
	entries, ok := hooks[event].([]any)
	if !ok {
		return false
	}
	for _, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if !ok {
			continue
		}
		rawHandlers, ok := entry["hooks"].([]any)
		if !ok {
			continue
		}
		for _, rawHandler := range rawHandlers {
			handler, ok := rawHandler.(map[string]any)
			if !ok {
				continue
			}
			if handler["type"] == "command" && handler["command"] == command {
				return true
			}
		}
	}
	return false
}

func projectionAppliedOfKind(t *testing.T, root, kind string) []schema.Event {
	t.Helper()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	var out []schema.Event
	for _, ev := range events {
		if ev.Type == EventProjectionApplied && projectionField(ev, "fragment") == kind {
			out = append(out, ev)
		}
	}
	return out
}

// Provenance is now emitted once per projection ACT by the Projection Envelope
// (see envelope_test.go), not per payload fragment — the old per-fragment
// idempotency test is superseded there.
