package setup

import (
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/internal/setup/assets"
)

func TestSkillAssetsDescribeTeamMemory(t *testing.T) {
	files := map[string][]byte{
		"cursor":   assets.CursorSkill,
		"claude":   assets.ClaudeSkill,
		"codex":    assets.CodexSkill,
		"hermes":   assets.HermesSkill,
		"openclaw": assets.OpenClawSkill,
		"pi":       assets.PiSkill,
		"nanobot":  assets.NanobotSkill,
		"nanoclaw": assets.NanoClawContainerSkill,
		"guide":    assets.ClaudeGuide,
	}
	for name, body := range files {
		text := string(body)
		for _, want := range []string{"team-shared brain", "owner_principal", "layer: org", "--local"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
	}
}
