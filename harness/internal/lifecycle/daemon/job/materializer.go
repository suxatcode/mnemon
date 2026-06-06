package job

import (
	"fmt"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/loader"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/trigger"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// SchemaVersion is the persisted job-document version stamped on every materialized
// job. The daemon package re-exports it as daemon.JobSchemaVersion.
const SchemaVersion = "mnemon.job.v1"

// Job is the one canonical daemon job. The materializer produces it with the
// lifecycle fields (Attempts/Lease/Error/Result) zero-valued; the daemon queue then
// persists and advances the same struct. The daemon package aliases it as daemon.Job
// (and daemon.Lease) so the queue's persistence/lease logic and the materializer
// share ONE struct instead of a Runtime/Job/jobFromRuntime triple.
type Job struct {
	SchemaVersion string         `json:"schema_version"`
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	ReactorID     string         `json:"reactor_id"`
	JobSpecRef    string         `json:"job_spec_ref,omitempty"`
	Target        map[string]any `json:"target"`
	Priority      string         `json:"priority"`
	Status        string         `json:"status"`
	DueAt         string         `json:"due_at"`
	Attempts      int            `json:"attempts"`
	MaxAttempts   int            `json:"max_attempts"`
	Lease         *Lease         `json:"lease,omitempty"`
	Budget        map[string]any `json:"budget,omitempty"`
	EvidenceRefs  []string       `json:"evidence_refs,omitempty"`
	CorrelationID string         `json:"correlation_id"`
	Error         map[string]any `json:"error,omitempty"`
	Result        map[string]any `json:"result,omitempty"`
	UpdatedAt     string         `json:"updated_at,omitempty"`
}

type Lease struct {
	OwnerID    string `json:"owner_id"`
	AcquiredAt string `json:"acquired_at"`
	ExpiresAt  string `json:"expires_at"`
	Renewals   int    `json:"renewals"`
}

func Materialize(def loader.Definition, decision trigger.Decision, now time.Time) ([]Job, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if len(decision.Events) == 0 {
		runtime, err := materializeOne(def, nil, now)
		if err != nil {
			return nil, err
		}
		return []Job{runtime}, nil
	}
	runtimes := make([]Job, 0, len(decision.Events))
	for i := range decision.Events {
		runtime, err := materializeOne(def, &decision.Events[i], now)
		if err != nil {
			return nil, err
		}
		runtimes = append(runtimes, runtime)
	}
	return runtimes, nil
}

func materializeOne(def loader.Definition, event *schema.Event, now time.Time) (Job, error) {
	jobType, reactorID, jobSpecRef, target, err := actionTarget(def)
	if err != nil {
		return Job{}, err
	}
	evidenceRefs := []string{}
	correlationID := "daemon:" + def.ID
	// No-event (cron/interval/threshold) jobs use a minute-bucketed suffix so a
	// trigger that stays true across a background tick burst dedups to one job
	// per minute (jobExistsAnyStatus keys on the exact id) instead of flooding
	// the queue once per distinct-second tick.
	suffix := now.UTC().Format("20060102T1504Z")
	if event != nil {
		evidenceRefs = append(evidenceRefs, event.ID)
		correlationID = event.CorrelationID
		suffix = event.ID
		target["source_event_id"] = event.ID
		target["event_type"] = event.Type
	}
	return Job{
		SchemaVersion: SchemaVersion,
		ID:            runtimeID(def.ID, suffix),
		Type:          jobType,
		ReactorID:     reactorID,
		JobSpecRef:    jobSpecRef,
		Target:        target,
		Priority:      "normal",
		Status:        "queued",
		DueAt:         now.UTC().Format(time.RFC3339),
		MaxAttempts:   budgetInt(def.Budget.MaxAttempts, 1),
		Budget:        budgetMap(def.Budget),
		EvidenceRefs:  evidenceRefs,
		CorrelationID: correlationID,
		UpdatedAt:     now.UTC().Format(time.RFC3339),
	}, nil
}

func actionTarget(def loader.Definition) (string, string, string, map[string]any, error) {
	switch {
	case def.Do.CLI != "":
		return "cli", def.ID, def.ID, map[string]any{
			"cli": def.Do.CLI,
			"cwd": def.Do.CWD,
			"env": def.Do.Env,
		}, nil
	case def.Do.Subagent != "":
		target := map[string]any{"subagent": def.Do.Subagent}
		if def.Do.PromptOverride != "" {
			target["prompt"] = def.Do.PromptOverride
		}
		if loop := semanticLoop(def); loop != "" {
			target["loop"] = loop
		}
		return "semantic", def.Do.Subagent, def.Do.Subagent, target, nil
	case def.Do.SpawnRunner != "":
		target := map[string]any{
			"runner_id":     def.Do.SpawnRunner,
			"prompt":        def.Do.Prompt,
			"isolated_home": boolValue(def.Do.IsolatedHome, true),
			"prompt_file":   def.Do.PromptFile,
		}
		if def.Do.MaxTurns > 0 {
			target["max_turns"] = def.Do.MaxTurns
		}
		return "spawn_runner", def.Do.SpawnRunner, def.ID, target, nil
	default:
		return "", "", "", nil, fmt.Errorf("daemon job %s has no materializable action", def.ID)
	}
}

func semanticLoop(def loader.Definition) string {
	if value, ok := def.Metadata["loop"].(string); ok {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	for _, candidate := range []string{def.ID, def.Do.Subagent} {
		if idx := strings.Index(candidate, "."); idx > 0 {
			return candidate[:idx]
		}
	}
	return ""
}

func budgetMap(budget loader.Budget) map[string]any {
	values := map[string]any{
		"cost_usd":     0.0,
		"max_sec":      budgetInt(budget.MaxSec, 300),
		"max_turns":    budgetInt(budget.MaxTurns, 3),
		"max_attempts": budgetInt(budget.MaxAttempts, 1),
		"concurrency":  budgetInt(budget.Concurrency, 1),
	}
	if budget.CostUSD != nil {
		values["cost_usd"] = *budget.CostUSD
	}
	return values
}

func runtimeID(id, suffix string) string {
	return "job_" + sanitize(id) + "_" + sanitize(suffix)
}

func sanitize(value string) string {
	replacer := strings.NewReplacer("/", "_", ":", "_", " ", "_")
	value = replacer.Replace(value)
	value = strings.Trim(value, "._-")
	if value == "" {
		return "unknown"
	}
	return value
}

func budgetInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
