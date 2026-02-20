package cmd

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/internal/setup"
	"github.com/spf13/cobra"
)

var (
	setupTarget string
	setupEject  bool
	setupYes    bool
	setupGlobal bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Deploy mnemon into LLM CLI environments",
	Long: `Detect installed LLM CLIs and deploy mnemon integration.

By default, installs to project-local config (.claude/, .openclaw/).
Use --global to install to user-wide config (~/.claude/, ~/.openclaw/).

Supported environments: Claude Code, OpenClaw.

Examples:
  mnemon setup                              # Interactive: project-local install
  mnemon setup --global                     # Interactive: user-wide install
  mnemon setup --target claude-code         # Non-interactive: Claude Code only
  mnemon setup --eject                      # Interactive: remove integrations
  mnemon setup --eject --target claude-code # Non-interactive: remove Claude Code only
  mnemon setup --yes                        # Auto-confirm all prompts`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().StringVar(&setupTarget, "target", "", "target environment (claude-code, openclaw)")
	setupCmd.Flags().BoolVar(&setupEject, "eject", false, "remove mnemon integrations")
	setupCmd.Flags().BoolVar(&setupYes, "yes", false, "auto-confirm all prompts (CI-friendly)")
	setupCmd.Flags().BoolVar(&setupGlobal, "global", false, "install to user-wide config (~/.claude/) instead of project-local (.claude/)")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	if setupTarget != "" && setupTarget != "claude-code" && setupTarget != "openclaw" {
		return fmt.Errorf("invalid target %q (must be claude-code or openclaw)", setupTarget)
	}

	envs := setup.DetectEnvironments(setupGlobal)

	if setupEject {
		return runEjectFlow(envs)
	}
	return runInstallFlow(envs)
}

func runInstallFlow(envs []setup.Environment) error {
	// Non-interactive with --target: install specific target directly
	if setupTarget != "" {
		for i := range envs {
			if envs[i].Name == setupTarget {
				return installEnv(&envs[i])
			}
		}
		return fmt.Errorf("unknown target %q", setupTarget)
	}

	// Detection display
	fmt.Println("Detecting LLM CLI environments...")
	fmt.Println()

	var detected []setup.Environment
	for _, env := range envs {
		setup.DetectionLine(env.Detected, env.Display, env.Version, env.ConfigDir)
		if env.Detected {
			detected = append(detected, env)
		}
	}

	if len(detected) == 0 {
		fmt.Println("\nNo supported LLM CLI environments detected.")
		fmt.Println("Install Claude Code or OpenClaw, then run 'mnemon setup' again.")
		return nil
	}

	// Select environment
	var selected []setup.Environment
	if setupYes {
		selected = detected
	} else if setup.IsInteractive() {
		options := make([]string, len(detected))
		for i, env := range detected {
			options[i] = env.Display
		}
		idx := setup.SelectOne("Select environment", options, 0)
		selected = []setup.Environment{detected[idx]}
	} else {
		// Non-interactive without --yes: install all detected
		selected = detected
	}

	if len(selected) == 0 {
		fmt.Println("\nNo environments selected.")
		return nil
	}

	// Install each selected environment
	var errCount int
	for i := range selected {
		if err := installEnv(&selected[i]); err != nil {
			errCount++
		}
	}

	fmt.Println()
	fmt.Println("Done! Restart your LLM CLI session to activate.")
	fmt.Println("Run 'mnemon setup --eject' to remove integrations.")

	if errCount > 0 {
		return fmt.Errorf("%d error(s) during setup", errCount)
	}
	return nil
}

func installEnv(env *setup.Environment) error {
	switch env.Name {
	case "claude-code":
		errs := setup.ClaudeInstall(env.ConfigDir)
		injectMarkdown("CLAUDE.md", env.Name,
			setup.ClaudeGuidanceSections(),
			"You have persistent memory via the `mnemon` CLI (see skill for command reference).",
			"Inject memory guidance into ./CLAUDE.md?")
		if len(errs) > 0 {
			return errs[0]
		}

	case "openclaw":
		errs := setup.OpenClawInstall(env.ConfigDir)
		injectMarkdown("AGENTS.md", env.Name,
			setup.OpenClawGuidanceSections(),
			"You have persistent memory via the `mnemon` CLI.",
			"Inject memory guidance into ./AGENTS.md?")
		if len(errs) > 0 {
			return errs[0]
		}
	}
	return nil
}

func injectMarkdown(filePath, envName string, sections []setup.GuidanceSection, header, promptMsg string) {
	if setupYes {
		// --yes: inject all default sections
		selected := make([]bool, len(sections))
		for i := range selected {
			selected[i] = true
		}
		template := setup.ComposeMemoryBlock(header, sections, selected, "")
		doInject(filePath, template)
		return
	}

	if !setup.IsInteractive() {
		return
	}

	// Step 1: Confirm injection
	if !setup.Confirm(promptMsg, true) {
		return
	}

	// Step 2: Default or Manual
	mode := setup.SelectOne("Guidance mode", []string{"Default (recommended)", "Manual — configure each section"}, 0)
	if mode == 0 {
		selected := make([]bool, len(sections))
		for i := range selected {
			selected[i] = true
		}
		template := setup.ComposeMemoryBlock(header, sections, selected, "")
		doInject(filePath, template)
		return
	}

	// Step 3: Select modules
	moduleOpts := []string{
		"Recall — auto-recall past memories",
		"Remember — what/when to remember",
	}
	moduleDefs := []bool{true, true}
	modules := setup.SelectMulti("Select guidance modules", moduleOpts, moduleDefs)

	var parts []setup.GuidanceSection

	// Configure Recall
	if modules[0] {
		parts = append(parts, setup.GuidanceSection{
			Label:   "Recall",
			Content: configureRecall(),
		})
	}

	// Configure Remember + Delegation
	if modules[1] {
		rememberContent := configureRemember()
		if rememberContent != "" {
			parts = append(parts, setup.GuidanceSection{
				Label:   "Remember",
				Content: rememberContent,
			})
		}

		// Delegation sub-option
		if setup.Confirm("Enable delegation (auto memory writes)?", true) {
			var delegationContent string
			if envName == "claude-code" {
				delegationContent = configureDelegation()
			} else {
				delegationContent = setup.OpenClawDelegationDefault()
			}
			parts = append(parts, setup.GuidanceSection{
				Label:   "Delegation",
				Content: delegationContent,
			})
		}
	}

	// Additional sections
	var customText string
	if setup.Confirm("Add additional guideline sections?", false) {
		customText = setup.ReadMultiLine("Enter custom guidance (blank line to finish):")
	}

	if len(parts) == 0 && customText == "" {
		return
	}

	selected := make([]bool, len(parts))
	for i := range selected {
		selected[i] = true
	}
	template := setup.ComposeMemoryBlock(header, parts, selected, customText)
	doInject(filePath, template)
}

func configureRecall() string {
	defaultText := setup.RecallGuidanceDefault()
	fmt.Println()
	setup.PrintPreview(defaultText)
	fmt.Println()
	if setup.Confirm("Accept default recall guidance?", true) {
		return defaultText
	}
	custom := setup.ReadMultiLine("Enter custom recall guidance (blank line to finish):")
	if custom == "" {
		return defaultText
	}
	return "### Recall — before responding\n\n" + custom
}

func configureRemember() string {
	types := setup.DefaultRememberTypes()

	options := make([]string, len(types)+1)
	defaults := make([]bool, len(types)+1)
	for i, t := range types {
		options[i] = t.Name + " (" + t.Detail + ")"
		defaults[i] = true
	}
	options[len(types)] = "Custom — add your own type"
	defaults[len(types)] = false

	choices := setup.SelectMulti("Select memory types", options, defaults)

	var customType string
	if choices[len(types)] {
		name := setup.ReadLine("Type name: ")
		if name != "" {
			detail := setup.ReadLine("Examples: ")
			if detail != "" {
				customType = fmt.Sprintf("**%s** (%s)", name, detail)
			} else {
				customType = fmt.Sprintf("**%s**", name)
			}
		}
	}

	return setup.ComposeRememberSection(types, choices[:len(types)], customType)
}

func configureDelegation() string {
	models := []string{"sonnet", "haiku"}
	idx := setup.SelectOne("Delegation model", []string{"sonnet (recommended)", "haiku"}, 0)
	return setup.ClaudeDelegationWithModel(models[idx])
}

func doInject(filePath string, template []byte) {
	if changed, err := setup.InjectMemoryBlock(filePath, template); err != nil {
		fmt.Printf("  Warning: could not inject into %s: %v\n", filePath, err)
	} else if changed {
		fmt.Printf("  Memory guidance injected into %s\n", filePath)
	} else {
		fmt.Printf("  Memory guidance already present in %s\n", filePath)
	}
}

func runEjectFlow(envs []setup.Environment) error {
	// Non-interactive with --target: eject specific target directly
	if setupTarget != "" {
		for i := range envs {
			if envs[i].Name == setupTarget {
				return ejectEnv(&envs[i])
			}
		}
		return fmt.Errorf("unknown target %q", setupTarget)
	}

	// Interactive: detect and select which to eject
	fmt.Println("Detecting LLM CLI environments...")
	fmt.Println()

	var installed []setup.Environment
	for _, env := range envs {
		setup.DetectionLine(env.Detected, env.Display, env.Version, env.ConfigDir)
		if env.Detected {
			installed = append(installed, env)
		}
	}

	if len(installed) == 0 {
		fmt.Println("\nNo environments detected.")
		return nil
	}

	// Select environment to eject
	var selected []setup.Environment
	if setupYes {
		selected = installed
	} else if setup.IsInteractive() {
		options := make([]string, len(installed))
		for i, env := range installed {
			options[i] = env.Display
		}
		idx := setup.SelectOne("Select environment to remove", options, 0)
		selected = []setup.Environment{installed[idx]}
	} else {
		selected = installed
	}

	if len(selected) == 0 {
		fmt.Println("\nNo environments selected.")
		return nil
	}

	var errCount int
	for i := range selected {
		if err := ejectEnv(&selected[i]); err != nil {
			errCount++
		}
	}

	fmt.Println()
	fmt.Println("Done! All selected integrations removed.")

	if errCount > 0 {
		return fmt.Errorf("%d error(s) during eject", errCount)
	}
	return nil
}

func ejectEnv(env *setup.Environment) error {
	switch env.Name {
	case "claude-code":
		errs := setup.ClaudeEject(env.ConfigDir)
		// Optionally eject memory block from ./CLAUDE.md
		ejectMarkdown("CLAUDE.md", "Remove memory guidance from ./CLAUDE.md?")
		if len(errs) > 0 {
			return errs[0]
		}

	case "openclaw":
		errs := setup.OpenClawEject(env.ConfigDir)
		// Optionally eject memory block from ./AGENTS.md
		ejectMarkdown("AGENTS.md", "Remove memory guidance from ./AGENTS.md?")
		if len(errs) > 0 {
			return errs[0]
		}
	}
	return nil
}

func ejectMarkdown(filePath string, prompt string) {
	if setupYes {
		if changed, err := setup.EjectMemoryBlock(filePath); err != nil {
			fmt.Printf("  Warning: could not clean %s: %v\n", filePath, err)
		} else if changed {
			fmt.Printf("  Memory guidance removed from %s\n", filePath)
		}
	} else if setup.IsInteractive() {
		if setup.Confirm(prompt, true) {
			if changed, err := setup.EjectMemoryBlock(filePath); err != nil {
				fmt.Printf("  Warning: could not clean %s: %v\n", filePath, err)
			} else if changed {
				fmt.Printf("  Memory guidance removed from %s\n", filePath)
			}
		}
	}
}
