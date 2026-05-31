package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type ReplayOptions struct {
	Tiers []int
	Now   time.Time
}

type ReplayResult struct {
	SchemaVersion int           `json:"schema_version"`
	ID            string        `json:"id"`
	Status        string        `json:"status"`
	Tiers         []int         `json:"tiers"`
	Checks        []ReplayCheck `json:"checks"`
	WrittenAt     string        `json:"written_at"`
	ReportPath    string        `json:"report_path"`
}

type ReplayCheck struct {
	Tier     int    `json:"tier"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
	Scenario string `json:"scenario,omitempty"`
	Suite    string `json:"suite,omitempty"`
}

func ReplayRegression(root string, opts ReplayOptions) (ReplayResult, error) {
	if root == "" {
		root = "."
	}
	root = filepath.Clean(root)
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tiers := normalizeReplayTiers(opts.Tiers)
	var checks []ReplayCheck
	for _, tier := range tiers {
		checks = append(checks, replayTier(root, tier)...)
	}
	status := "pass"
	for _, check := range checks {
		if check.Status != "pass" {
			status = "fail"
			break
		}
	}
	result := ReplayResult{
		SchemaVersion: 1,
		ID:            "replay-" + now.UTC().Format("20060102T150405Z"),
		Status:        status,
		Tiers:         tiers,
		Checks:        checks,
		WrittenAt:     now.UTC().Format(time.RFC3339),
	}
	result.ReportPath = replayReportPath(root, result.ID)
	if err := writeReplayReport(root, result); err != nil {
		return ReplayResult{}, err
	}
	return result, nil
}

func replayTier(root string, tier int) []ReplayCheck {
	switch tier {
	case 1:
		return replaySuite(root, tier, "smoke")
	case 2:
		return replaySuite(root, tier, "regression")
	default:
		return []ReplayCheck{{
			Tier:    tier,
			Name:    "tier.supported",
			Status:  "fail",
			Message: fmt.Sprintf("unsupported regression replay tier %d", tier),
		}}
	}
}

func replaySuite(root string, tier int, suiteName string) []ReplayCheck {
	suite, err := LoadSuite(root, suiteName)
	if err != nil {
		return []ReplayCheck{{
			Tier:    tier,
			Name:    "suite.load",
			Status:  "fail",
			Suite:   suiteName,
			Message: err.Error(),
		}}
	}
	checks := []ReplayCheck{{
		Tier:    tier,
		Name:    "suite.load",
		Status:  "pass",
		Suite:   suite.Name,
		Message: suite.Source,
	}}
	for _, scenarioID := range suiteScenarioIDs(suite) {
		checks = append(checks, replayScenario(root, tier, suite.Name, scenarioID))
	}
	return checks
}

func replayScenario(root string, tier int, suiteName, scenarioID string) ReplayCheck {
	if _, err := BuildRunPlan(root, suiteName, scenarioID); err != nil {
		return ReplayCheck{
			Tier:     tier,
			Name:     "scenario.plan",
			Status:   "fail",
			Suite:    suiteName,
			Scenario: scenarioID,
			Message:  err.Error(),
		}
	}
	if _, found, err := LoadScenario(root, scenarioID); err != nil {
		return ReplayCheck{
			Tier:     tier,
			Name:     "scenario.catalog",
			Status:   "fail",
			Suite:    suiteName,
			Scenario: scenarioID,
			Message:  err.Error(),
		}
	} else if !found && !scenarioMarkdownExists(root, scenarioID) {
		return ReplayCheck{
			Tier:     tier,
			Name:     "scenario.exists",
			Status:   "fail",
			Suite:    suiteName,
			Scenario: scenarioID,
			Message:  "scenario not found in catalog JSON or markdown scenario path",
		}
	}
	return ReplayCheck{
		Tier:     tier,
		Name:     "scenario.plan",
		Status:   "pass",
		Suite:    suiteName,
		Scenario: scenarioID,
	}
}

func scenarioMarkdownExists(root, scenarioID string) bool {
	path := filepath.Join(root, "harness", "loops", "eval", "scenarios", filepath.FromSlash(scenarioID)+".md")
	_, err := os.Stat(path)
	return err == nil
}

func replayReportPath(root, id string) string {
	return filepath.ToSlash(filepath.Join(root, ".mnemon", "harness", "reports", "regression", id+".json"))
}

func writeReplayReport(root string, result ReplayResult) error {
	path := filepath.FromSlash(result.ReportPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return nil
}

func normalizeReplayTiers(tiers []int) []int {
	if len(tiers) == 0 {
		return []int{1}
	}
	seen := map[int]bool{}
	var out []int
	for _, tier := range tiers {
		if seen[tier] {
			continue
		}
		seen[tier] = true
		out = append(out, tier)
	}
	sort.Ints(out)
	return out
}
