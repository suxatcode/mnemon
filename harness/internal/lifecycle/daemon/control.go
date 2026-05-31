package daemon

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/loader"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

type PauseState struct {
	SchemaVersion int    `json:"schema_version"`
	Paused        bool   `json:"paused"`
	Reason        string `json:"reason,omitempty"`
	Since         string `json:"since,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

type BudgetSnapshot struct {
	UsedUSDToday       float64  `json:"used_usd_today"`
	DailyCostUSD       *float64 `json:"daily_cost_usd,omitempty"`
	CostRemainingUSD   *float64 `json:"cost_remaining_usd,omitempty"`
	RealTurnsToday     int      `json:"real_turns_today"`
	DailyRealTurns     int      `json:"daily_real_turns,omitempty"`
	RealTurnsRemaining int      `json:"real_turns_remaining,omitempty"`
	Enforced           bool     `json:"enforced"`
}

type EnabledJobSnapshot struct {
	ID      string `json:"id"`
	Trigger string `json:"trigger"`
	Action  string `json:"action"`
	Source  string `json:"source,omitempty"`
}

type StatusSnapshot struct {
	SchemaVersion int                  `json:"schema_version"`
	TS            string               `json:"ts"`
	Paused        PauseState           `json:"paused"`
	QueueDepth    QueueDepth           `json:"queue_depth"`
	Budget        BudgetSnapshot       `json:"budget"`
	RecentTicks   []TickLogRecord      `json:"recent_ticks"`
	EnabledJobs   []EnabledJobSnapshot `json:"enabled_jobs"`
}

func Pause(root, reason string, now time.Time) (PauseState, error) {
	if reason == "" {
		reason = "manual"
	}
	state := PauseState{
		SchemaVersion: 1,
		Paused:        true,
		Reason:        reason,
		Since:         normalizeControlTime(now).Format(time.RFC3339),
		UpdatedAt:     normalizeControlTime(now).Format(time.RFC3339),
	}
	if err := writePauseState(root, state); err != nil {
		return PauseState{}, err
	}
	if err := appendControlEvent(root, "daemon.paused", reason, state, normalizeControlTime(now)); err != nil {
		return PauseState{}, err
	}
	return state, nil
}

func Resume(root string, now time.Time) (PauseState, error) {
	state := PauseState{
		SchemaVersion: 1,
		Paused:        false,
		Reason:        "manual_resume",
		UpdatedAt:     normalizeControlTime(now).Format(time.RFC3339),
	}
	if err := writePauseState(root, state); err != nil {
		return PauseState{}, err
	}
	if err := appendControlEvent(root, "daemon.resumed", "manual_resume", state, normalizeControlTime(now)); err != nil {
		return PauseState{}, err
	}
	return state, nil
}

func IsPaused(root string) (PauseState, error) {
	paths, err := layout.Resolve(root)
	if err != nil {
		return PauseState{}, err
	}
	return readPauseState(paths)
}

func Inspect(root string, limit int) (StatusSnapshot, error) {
	if limit <= 0 {
		limit = 10
	}
	d, err := New(root, Options{})
	if err != nil {
		return StatusSnapshot{}, err
	}
	now := time.Now().UTC()
	paused, err := d.pauseState()
	if err != nil {
		return StatusSnapshot{}, err
	}
	depth, err := d.queueDepth()
	if err != nil {
		return StatusSnapshot{}, err
	}
	budget, err := d.budgetSnapshot(now)
	if err != nil {
		return StatusSnapshot{}, err
	}
	ticks, err := recentTicks(d.paths, limit)
	if err != nil {
		return StatusSnapshot{}, err
	}
	jobs, err := enabledJobs(d.paths.Root)
	if err != nil {
		return StatusSnapshot{}, err
	}
	return StatusSnapshot{
		SchemaVersion: 1,
		TS:            now.Format(time.RFC3339),
		Paused:        paused,
		QueueDepth:    depth,
		Budget:        budget,
		RecentTicks:   ticks,
		EnabledJobs:   jobs,
	}, nil
}

func (d *Daemon) pauseState() (PauseState, error) {
	return readPauseState(d.paths)
}

func (d *Daemon) budgetSnapshot(now time.Time) (BudgetSnapshot, error) {
	catalog, err := d.LoadCatalog()
	if err != nil {
		return BudgetSnapshot{}, err
	}
	used, err := jobCostUsedToday(d.paths, now)
	if err != nil {
		return BudgetSnapshot{}, err
	}
	turns, err := realTurnsUsedToday(d.paths, now)
	if err != nil {
		return BudgetSnapshot{}, err
	}
	snapshot := BudgetSnapshot{
		UsedUSDToday:   used,
		DailyCostUSD:   catalog.GlobalBudget.DailyCostUSD,
		RealTurnsToday: turns,
		DailyRealTurns: catalog.GlobalBudget.DailyRealTurns,
		Enforced:       catalog.GlobalBudget.Enabled,
	}
	if catalog.GlobalBudget.DailyCostUSD != nil {
		remaining := *catalog.GlobalBudget.DailyCostUSD - used
		if remaining < 0 {
			remaining = 0
		}
		snapshot.CostRemainingUSD = &remaining
	}
	if catalog.GlobalBudget.DailyRealTurns > 0 {
		snapshot.RealTurnsRemaining = max(0, catalog.GlobalBudget.DailyRealTurns-turns)
	}
	return snapshot, nil
}

func (d *Daemon) budgetExceeded(now time.Time) (bool, string, error) {
	snapshot, err := d.budgetSnapshot(now)
	if err != nil {
		return false, "", err
	}
	if !snapshot.Enforced {
		return false, "", nil
	}
	if snapshot.DailyCostUSD != nil && snapshot.UsedUSDToday >= *snapshot.DailyCostUSD {
		return true, fmt.Sprintf("daily cost budget exhausted: %.4f/%.4f USD", snapshot.UsedUSDToday, *snapshot.DailyCostUSD), nil
	}
	if snapshot.DailyRealTurns > 0 && snapshot.RealTurnsToday >= snapshot.DailyRealTurns {
		return true, fmt.Sprintf("daily real-turn budget exhausted: %d/%d", snapshot.RealTurnsToday, snapshot.DailyRealTurns), nil
	}
	return false, "", nil
}

func readPauseState(paths layout.Paths) (PauseState, error) {
	var state PauseState
	if err := readJSON(pausePath(paths), &state); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PauseState{SchemaVersion: 1, Paused: false}, nil
		}
		return PauseState{}, err
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = 1
	}
	return state, nil
}

func writePauseState(root string, state PauseState) error {
	paths, err := layout.EnsureProject(root)
	if err != nil {
		return err
	}
	return writeJSONAtomic(pausePath(paths), state)
}

func pausePath(paths layout.Paths) string {
	return filepath.Join(paths.HarnessDir, "daemon", "pause.json")
}

func appendControlEvent(root, eventType, reason string, state PauseState, now time.Time) error {
	store, err := eventlog.New(root)
	if err != nil {
		return err
	}
	return store.Append(schema.Event{
		SchemaVersion: schema.Version,
		ID:            fmt.Sprintf("evt_%s_%d", cleanEventToken(eventType), now.UnixNano()),
		TS:            now.Format(time.RFC3339),
		Type:          eventType,
		Actor:         "mnemon-daemon",
		Source:        "daemon.control",
		CorrelationID: "daemon:control",
		CausedBy:      nil,
		Payload: map[string]any{
			"reason": reason,
			"paused": state.Paused,
		},
	})
}

func recentTicks(paths layout.Paths, limit int) ([]TickLogRecord, error) {
	path := filepath.Join(paths.HarnessDir, "daemon", "tick-log.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	var records []TickLogRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record TickLogRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(records) > limit {
		records = records[len(records)-limit:]
	}
	return records, nil
}

func enabledJobs(root string) ([]EnabledJobSnapshot, error) {
	catalog, err := loader.Load(root, loader.Options{AcknowledgeModelCost: true})
	if err != nil {
		return nil, err
	}
	jobs := make([]EnabledJobSnapshot, 0, len(catalog.Jobs))
	for _, def := range catalog.Jobs {
		if !def.IsEnabled() {
			continue
		}
		jobs = append(jobs, EnabledJobSnapshot{
			ID:      def.ID,
			Trigger: triggerSummary(def.When),
			Action:  actionKind(def),
			Source:  def.Source.Kind,
		})
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	return jobs, nil
}

func jobCostUsedToday(paths layout.Paths, now time.Time) (float64, error) {
	var total float64
	for _, status := range []string{"completed", "failed"} {
		dir := filepath.Join(paths.JobsDir, status)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			var job Job
			if err := readJSON(filepath.Join(dir, entry.Name()), &job); err != nil {
				return 0, err
			}
			if !sameUTCDay(job.UpdatedAt, now) {
				continue
			}
			total += budgetFloat(job.Budget, "cost_usd")
		}
	}
	return total, nil
}

func realTurnsUsedToday(paths layout.Paths, now time.Time) (int, error) {
	records, err := recentTicks(paths, 100000)
	if err != nil {
		return 0, err
	}
	var total int
	for _, record := range records {
		if record.Status != "completed" || !sameUTCDay(record.TS, now) {
			continue
		}
		total += record.RealTurnsUsed
	}
	return total, nil
}

func sameUTCDay(ts string, now time.Time) bool {
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false
	}
	parsed = parsed.UTC()
	now = now.UTC()
	return parsed.Year() == now.Year() && parsed.YearDay() == now.YearDay()
}

func triggerSummary(trigger loader.Trigger) string {
	switch {
	case trigger.Event != "":
		return "event"
	case trigger.Cron != "":
		return "cron"
	case trigger.Interval != "":
		return "interval"
	case trigger.Threshold != nil:
		return "threshold"
	case len(trigger.Any) > 0:
		return "composite:any"
	case len(trigger.All) > 0:
		return "composite:all"
	default:
		return "unknown"
	}
}

func actionKind(def loader.Definition) string {
	switch {
	case def.Do.CLI != "":
		return "cli"
	case def.Do.Subagent != "":
		return "subagent"
	case def.Do.SpawnRunner != "":
		return "spawn_runner"
	default:
		return "unknown"
	}
}

func normalizeControlTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func budgetFloat(budget map[string]any, key string) float64 {
	value, ok := budget[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}
