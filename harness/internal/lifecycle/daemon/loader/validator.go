package loader

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/metric"
)

var daemonJobID = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

type validateContext struct {
	globalBudget          GlobalBudget
	acknowledgeModelCost  bool
	checkSpawnRunnerGate  bool
	allowLiftedController bool
	sourcePath            string
}

func validateDefinition(def *Definition, ctx validateContext) ([]string, error) {
	var warnings []string
	if strings.TrimSpace(def.ID) == "" {
		return nil, fmt.Errorf("daemon job missing id: %s", ctx.sourcePath)
	}
	if !daemonJobID.MatchString(def.ID) {
		return nil, fmt.Errorf("daemon job %q has invalid id characters: %s", def.ID, ctx.sourcePath)
	}
	if err := validateTrigger(def.When, 0); err != nil {
		return nil, fmt.Errorf("daemon job %s invalid trigger: %w", def.ID, err)
	}
	if err := validateAction(def.Do); err != nil {
		return nil, fmt.Errorf("daemon job %s invalid action: %w", def.ID, err)
	}
	if def.Do.SpawnRunner != "" && ctx.checkSpawnRunnerGate && !ctx.acknowledgeModelCost {
		warnings = append(warnings, fmt.Sprintf("daemon job %s disabled: spawn_runner requires model-cost acknowledgement", def.ID))
		def.SetEnabled(false)
	}
	if def.Budget.CostUSD != nil && ctx.globalBudget.Enabled && ctx.globalBudget.DailyCostUSD != nil && *def.Budget.CostUSD > *ctx.globalBudget.DailyCostUSD {
		warnings = append(warnings, fmt.Sprintf("daemon job %s budget.cost_usd exceeds global daily_cost_usd", def.ID))
	}
	return warnings, nil
}

func validateTrigger(trigger Trigger, depth int) error {
	if depth > 3 {
		return fmt.Errorf("composite trigger nesting depth exceeds 3")
	}
	kinds := 0
	if trigger.Event != "" {
		kinds++
	}
	if trigger.Cron != "" {
		kinds++
		if err := validateCron(trigger.Cron); err != nil {
			return err
		}
	}
	if trigger.Interval != "" {
		kinds++
		if _, err := time.ParseDuration(trigger.Interval); err != nil {
			return fmt.Errorf("invalid interval %q: %w", trigger.Interval, err)
		}
	}
	if trigger.Threshold != nil {
		kinds++
		if err := validateThreshold(*trigger.Threshold); err != nil {
			return err
		}
	}
	if len(trigger.Any) > 0 {
		kinds++
		for _, child := range trigger.Any {
			if err := validateTrigger(child, depth+1); err != nil {
				return err
			}
		}
	}
	if len(trigger.All) > 0 {
		kinds++
		for _, child := range trigger.All {
			if err := validateTrigger(child, depth+1); err != nil {
				return err
			}
		}
	}
	if kinds == 0 {
		return fmt.Errorf("must include at least one trigger kind")
	}
	if kinds > 1 {
		return fmt.Errorf("must include exactly one trigger kind")
	}
	return nil
}

func validateAction(action Action) error {
	kinds := 0
	for _, value := range []string{action.Subagent, action.CLI, action.SpawnRunner} {
		if value != "" {
			kinds++
		}
	}
	if kinds != 1 {
		return fmt.Errorf("must include exactly one action kind")
	}
	return nil
}

func validateCron(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron %q must have 5 fields", expr)
	}
	for _, field := range fields {
		if field == "" {
			return fmt.Errorf("cron %q has an empty field", expr)
		}
		if err := validateCronField(field); err != nil {
			return fmt.Errorf("cron %q: %w", expr, err)
		}
	}
	return nil
}

// validateCronField rejects cron field syntax the runtime evaluator cannot match
// (so a bad expression is caught at load/dry-run, not at tick time). Grammar:
// "*", "*/step", "n", "lo-hi", "lo-hi/step", "n/step", and comma lists thereof.
func validateCronField(field string) error {
	for _, part := range strings.Split(field, ",") {
		base := part
		if i := strings.Index(part, "/"); i >= 0 {
			base = part[:i]
			if step, err := strconv.Atoi(part[i+1:]); err != nil || step <= 0 {
				return fmt.Errorf("invalid cron step %q", part)
			}
		}
		if base == "*" {
			continue
		}
		if i := strings.Index(base, "-"); i >= 0 {
			lo, err1 := strconv.Atoi(base[:i])
			hi, err2 := strconv.Atoi(base[i+1:])
			if err1 != nil || err2 != nil || lo > hi {
				return fmt.Errorf("invalid cron range %q", base)
			}
			continue
		}
		if _, err := strconv.Atoi(base); err != nil {
			return fmt.Errorf("invalid cron field %q", part)
		}
	}
	return nil
}

func validateThreshold(threshold Threshold) error {
	if !metric.IsKnown(threshold.Metric) {
		return fmt.Errorf("unknown threshold metric %q", threshold.Metric)
	}
	switch threshold.Op {
	case ">", ">=", "<", "<=", "==", "!=":
	default:
		return fmt.Errorf("invalid threshold op %q", threshold.Op)
	}
	if threshold.Window != "" {
		if _, err := time.ParseDuration(threshold.Window); err != nil {
			return fmt.Errorf("invalid threshold window %q: %w", threshold.Window, err)
		}
	}
	return nil
}
