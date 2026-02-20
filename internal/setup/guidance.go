package setup

import (
	"fmt"
	"strings"
)

// GuidanceSection represents a selectable guidance section for memory injection.
type GuidanceSection struct {
	Label       string
	Description string
	Content     string
}

// RememberType represents a selectable memory category for the Remember section.
type RememberType struct {
	Name   string // e.g., "user directive"
	Detail string // e.g., "preference, decision, correction"
}

var recallGuidance = "### Recall — before responding\n\n" +
	"When you see `[Past memory]` in your context, **use it**. Reference relevant memories rather than re-deriving.\n\n" +
	"If no memories were injected but the topic could benefit from past context, run `mnemon recall \"<topic>\" --limit 5` yourself.\n\n" +
	"Do NOT recall for: operational commands, short confirmations, or follow-up within the same topic already in context.\n"

var rememberGuidance = "### Remember — after responding\n\n" +
	"Ask: **if I forget this, will the user have to repeat themselves or will I redo significant work?**\n\n" +
	"Three types qualify: **user directive** (preference, decision, correction), **reasoning conclusion** (non-trivial analysis, design evaluation), **observed state** (system fact, environment detail).\n"

var claudeDelegation = "If yes, **delegate to a Task sub-agent** (`subagent_type=\"Bash\"`, `model=\"sonnet\"`). " +
	"Only provide what to store — content, category, importance, entities. " +
	"The sub-agent will read the mnemon skill and execute the correct commands itself.\n\n" +
	"Do NOT: write CLI commands or workflow steps in the sub-agent prompt " +
	"(the sub-agent has access to the skill docs and will use the correct flags). " +
	"Do NOT run memory writes in the main conversation, or remember operational/public/git-tracked/transient info.\n"

var openclawDelegation = "If yes, store it: `mnemon remember \"<fact>\" --cat <cat> --imp <1-5> --entities \"e1,e2\" --source agent`\n\n" +
	"Do NOT remember operational/public/git-tracked/transient info.\n"

// DefaultRememberTypes returns the 3 default memory categories.
func DefaultRememberTypes() []RememberType {
	return []RememberType{
		{"user directive", "preference, decision, correction"},
		{"reasoning conclusion", "non-trivial analysis, design evaluation"},
		{"observed state", "system fact, environment detail"},
	}
}

var typeCountWords = []string{"", "One type qualifies", "Two types qualify", "Three types qualify", "Four types qualify", "Five types qualify"}

// ComposeRememberSection builds the Remember section from selected types and optional custom type.
func ComposeRememberSection(types []RememberType, selected []bool, customType string) string {
	var parts []string
	for i, t := range types {
		if i < len(selected) && selected[i] {
			parts = append(parts, fmt.Sprintf("**%s** (%s)", t.Name, t.Detail))
		}
	}
	if customType != "" {
		parts = append(parts, customType)
	}
	if len(parts) == 0 {
		return ""
	}

	var prefix string
	if len(parts) < len(typeCountWords) {
		prefix = typeCountWords[len(parts)]
	} else {
		prefix = fmt.Sprintf("%d types qualify", len(parts))
	}

	return "### Remember — after responding\n\n" +
		"Ask: **if I forget this, will the user have to repeat themselves or will I redo significant work?**\n\n" +
		prefix + ": " + strings.Join(parts, ", ") + ".\n"
}

// ClaudeDelegationWithModel returns the Claude Code delegation section with the specified model.
func ClaudeDelegationWithModel(model string) string {
	return fmt.Sprintf("If yes, **delegate to a Task sub-agent** (`subagent_type=\"Bash\"`, `model=\"%s\"`). ", model) +
		"Only provide what to store — content, category, importance, entities. " +
		"The sub-agent will read the mnemon skill and execute the correct commands itself.\n\n" +
		"Do NOT: write CLI commands or workflow steps in the sub-agent prompt " +
		"(the sub-agent has access to the skill docs and will use the correct flags). " +
		"Do NOT run memory writes in the main conversation, or remember operational/public/git-tracked/transient info.\n"
}

// ClaudeGuidanceSections returns the selectable guidance sections for Claude Code.
func ClaudeGuidanceSections() []GuidanceSection {
	return []GuidanceSection{
		{
			Label:       "Recall",
			Description: "auto-recall past memories",
			Content:     recallGuidance,
		},
		{
			Label:       "Remember",
			Description: "what/when to remember",
			Content:     rememberGuidance,
		},
		{
			Label:       "Delegation",
			Description: "sub-agent write pattern",
			Content:     claudeDelegation,
		},
	}
}

// OpenClawGuidanceSections returns the selectable guidance sections for OpenClaw.
func OpenClawGuidanceSections() []GuidanceSection {
	return []GuidanceSection{
		{
			Label:       "Recall",
			Description: "auto-recall past memories",
			Content:     recallGuidance,
		},
		{
			Label:       "Remember",
			Description: "what/when to remember",
			Content:     rememberGuidance,
		},
		{
			Label:       "Delegation",
			Description: "direct CLI write command",
			Content:     openclawDelegation,
		},
	}
}

// RecallGuidanceDefault returns the default recall guidance text.
func RecallGuidanceDefault() string {
	return recallGuidance
}

// OpenClawDelegationDefault returns the default OpenClaw delegation text.
func OpenClawDelegationDefault() string {
	return openclawDelegation
}

// ComposeMemoryBlock builds a memory guidance block from selected sections and optional custom text.
func ComposeMemoryBlock(header string, sections []GuidanceSection, selected []bool, customText string) []byte {
	var b strings.Builder
	b.WriteString("<!-- mnemon:start -->\n")
	b.WriteString("## Memory\n\n")
	b.WriteString(header)
	b.WriteString("\n")

	for i, sec := range sections {
		if i < len(selected) && selected[i] && sec.Content != "" {
			b.WriteString("\n")
			b.WriteString(sec.Content)
		}
	}

	if customText != "" {
		b.WriteString("\n")
		b.WriteString(customText)
		if !strings.HasSuffix(customText, "\n") {
			b.WriteString("\n")
		}
	}

	b.WriteString("<!-- mnemon:end -->\n")
	return []byte(b.String())
}
