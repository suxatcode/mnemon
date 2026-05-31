package loader

import "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/trigger"

type Catalog struct {
	Jobs         []Definition
	GlobalBudget GlobalBudget
	Warnings     []string
}

type Source struct {
	Path       string
	Kind       string
	Loop       string
	Controller string
}

type Definition struct {
	ID          string         `json:"id" yaml:"id"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty"`
	When        Trigger        `json:"when" yaml:"when"`
	Do          Action         `json:"do" yaml:"do"`
	Budget      Budget         `json:"budget,omitempty" yaml:"budget,omitempty"`
	Enabled     *bool          `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	Source      Source         `json:"source,omitempty" yaml:"-"`
}

func (d Definition) IsEnabled() bool {
	return d.Enabled == nil || *d.Enabled
}

func (d *Definition) SetEnabled(value bool) {
	d.Enabled = &value
}

type Trigger = trigger.Spec
type Threshold = trigger.Threshold

type Action struct {
	Subagent       string            `json:"subagent,omitempty" yaml:"subagent,omitempty"`
	PromptOverride string            `json:"prompt_override,omitempty" yaml:"prompt_override,omitempty"`
	CLI            string            `json:"cli,omitempty" yaml:"cli,omitempty"`
	CWD            string            `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	SpawnRunner    string            `json:"spawn_runner,omitempty" yaml:"spawn_runner,omitempty"`
	Prompt         string            `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	IsolatedHome   *bool             `json:"isolated_home,omitempty" yaml:"isolated_home,omitempty"`
	MaxTurns       int               `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`
	PromptFile     string            `json:"prompt_file,omitempty" yaml:"prompt_file,omitempty"`
}

type Budget struct {
	CostUSD     *float64 `json:"cost_usd,omitempty" yaml:"cost_usd,omitempty"`
	MaxSec      int      `json:"max_sec,omitempty" yaml:"max_sec,omitempty"`
	MaxTurns    int      `json:"max_turns,omitempty" yaml:"max_turns,omitempty"`
	MaxAttempts int      `json:"max_attempts,omitempty" yaml:"max_attempts,omitempty"`
	Concurrency int      `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
}

type GlobalConfig struct {
	GlobalBudget GlobalBudget `json:"global_budget" yaml:"global_budget"`
}

type GlobalBudget struct {
	DailyCostUSD   *float64 `json:"daily_cost_usd,omitempty" yaml:"daily_cost_usd,omitempty"`
	DailyRealTurns int      `json:"daily_real_turns,omitempty" yaml:"daily_real_turns,omitempty"`
	Enabled        bool     `json:"enabled,omitempty" yaml:"enabled,omitempty"`
}
