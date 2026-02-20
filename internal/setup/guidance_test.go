package setup

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestComposeMatchesClaude(t *testing.T) {
	sections := ClaudeGuidanceSections()
	selected := []bool{true, true, true}
	composed := ComposeMemoryBlock(
		"You have persistent memory via the `mnemon` CLI (see skill for command reference).",
		sections, selected, "")

	original, err := os.ReadFile("assets/claude/claude_memory.md")
	if err != nil {
		t.Fatalf("read original: %v", err)
	}

	if string(composed) != string(original) {
		t.Errorf("MISMATCH\n=== COMPOSED (%d bytes) ===\n%s\n=== ORIGINAL (%d bytes) ===\n%s",
			len(composed), composed, len(original), original)
	}
}

func TestComposeMatchesOpenClaw(t *testing.T) {
	sections := OpenClawGuidanceSections()
	selected := []bool{true, true, true}
	composed := ComposeMemoryBlock(
		"You have persistent memory via the `mnemon` CLI.",
		sections, selected, "")

	original, err := os.ReadFile("assets/openclaw/openclaw_memory.md")
	if err != nil {
		t.Fatalf("read original: %v", err)
	}

	if string(composed) != string(original) {
		t.Errorf("MISMATCH\n=== COMPOSED (%d bytes) ===\n%s\n=== ORIGINAL (%d bytes) ===\n%s",
			len(composed), composed, len(original), original)
	}
}

func TestComposePartialSelection(t *testing.T) {
	sections := ClaudeGuidanceSections()
	selected := []bool{true, false, false}
	composed := string(ComposeMemoryBlock(
		"You have persistent memory via the `mnemon` CLI (see skill for command reference).",
		sections, selected, ""))

	if !strings.Contains(composed, "### Recall") {
		t.Error("expected Recall section")
	}
	if strings.Contains(composed, "### Remember") {
		t.Error("unexpected Remember section")
	}
	if strings.Contains(composed, "delegate to a Task sub-agent") {
		t.Error("unexpected Delegation section")
	}
	if !strings.HasPrefix(composed, "<!-- mnemon:start -->") {
		t.Error("missing start marker")
	}
	if !strings.HasSuffix(composed, "<!-- mnemon:end -->\n") {
		t.Error("missing end marker")
	}
}

func TestComposeWithCustomText(t *testing.T) {
	sections := ClaudeGuidanceSections()
	selected := []bool{true, false, false}
	custom := "### My Custom Rule\n\nAlways use formal tone.\n"
	composed := string(ComposeMemoryBlock(
		"You have persistent memory via the `mnemon` CLI (see skill for command reference).",
		sections, selected, custom))

	if !strings.Contains(composed, "### Recall") {
		t.Error("expected Recall section")
	}
	if !strings.Contains(composed, "### My Custom Rule") {
		t.Error("expected custom section")
	}
	if !strings.Contains(composed, "Always use formal tone.") {
		t.Error("expected custom content")
	}
}

func TestComposeCustomOnly(t *testing.T) {
	sections := ClaudeGuidanceSections()
	selected := []bool{false, false, false}
	custom := "Only remember user corrections.\n"
	composed := string(ComposeMemoryBlock(
		"You have persistent memory via the `mnemon` CLI (see skill for command reference).",
		sections, selected, custom))

	if strings.Contains(composed, "### Recall") {
		t.Error("unexpected Recall section")
	}
	if !strings.Contains(composed, "Only remember user corrections.") {
		t.Error("expected custom content")
	}
}

func TestComposeSkipsEmptyContent(t *testing.T) {
	sections := []GuidanceSection{
		{Label: "Recall", Content: recallGuidance},
		{Label: "Remember", Content: ""}, // empty — should be skipped
	}
	selected := []bool{true, true}
	composed := string(ComposeMemoryBlock("Header.", sections, selected, ""))

	if !strings.Contains(composed, "### Recall") {
		t.Error("expected Recall section")
	}
	// Empty content should not produce double newlines
	if strings.Contains(composed, "\n\n\n\n") {
		t.Error("unexpected extra blank lines from empty content")
	}
}

func TestComposeRememberSectionAllTypes(t *testing.T) {
	types := DefaultRememberTypes()
	selected := []bool{true, true, true}
	composed := ComposeRememberSection(types, selected, "")

	if composed != rememberGuidance {
		t.Errorf("all types should match default remember guidance\n=== GOT ===\n%s\n=== WANT ===\n%s",
			composed, rememberGuidance)
	}
}

func TestComposeRememberSectionPartial(t *testing.T) {
	types := DefaultRememberTypes()
	selected := []bool{true, false, true}
	composed := ComposeRememberSection(types, selected, "")

	if !strings.Contains(composed, "Two types qualify") {
		t.Error("expected 'Two types qualify'")
	}
	if !strings.Contains(composed, "**user directive**") {
		t.Error("expected user directive")
	}
	if strings.Contains(composed, "**reasoning conclusion**") {
		t.Error("unexpected reasoning conclusion")
	}
	if !strings.Contains(composed, "**observed state**") {
		t.Error("expected observed state")
	}
}

func TestComposeRememberSectionWithCustomType(t *testing.T) {
	types := DefaultRememberTypes()
	selected := []bool{true, true, true}
	custom := "**tool preference** (IDE, formatter, linter)"
	composed := ComposeRememberSection(types, selected, custom)

	if !strings.Contains(composed, "Four types qualify") {
		t.Error("expected 'Four types qualify'")
	}
	if !strings.Contains(composed, custom) {
		t.Error("expected custom type in output")
	}
}

func TestComposeRememberSectionCustomOnly(t *testing.T) {
	types := DefaultRememberTypes()
	selected := []bool{false, false, false}
	custom := "**my type** (examples)"
	composed := ComposeRememberSection(types, selected, custom)

	if !strings.Contains(composed, "One type qualifies") {
		t.Error("expected 'One type qualifies'")
	}
	if !strings.Contains(composed, custom) {
		t.Error("expected custom type")
	}
}

func TestComposeRememberSectionEmpty(t *testing.T) {
	types := DefaultRememberTypes()
	selected := []bool{false, false, false}
	composed := ComposeRememberSection(types, selected, "")

	if composed != "" {
		t.Errorf("expected empty string for no types, got: %q", composed)
	}
}

func TestClaudeDelegationWithModel(t *testing.T) {
	for _, model := range []string{"sonnet", "haiku"} {
		composed := ClaudeDelegationWithModel(model)
		expected := fmt.Sprintf("`model=\"%s\"`", model)
		if !strings.Contains(composed, expected) {
			t.Errorf("expected %s in delegation, got: %s", expected, composed)
		}
	}
}

func TestClaudeDelegationSonnetMatchesDefault(t *testing.T) {
	composed := ClaudeDelegationWithModel("sonnet")
	if composed != claudeDelegation {
		t.Errorf("sonnet delegation should match default\n=== GOT ===\n%s\n=== WANT ===\n%s",
			composed, claudeDelegation)
	}
}
