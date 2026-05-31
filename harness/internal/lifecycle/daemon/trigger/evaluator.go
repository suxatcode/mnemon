package trigger

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/metric"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

type Spec struct {
	Event        string         `json:"event,omitempty" yaml:"event,omitempty"`
	PayloadMatch map[string]any `json:"payload_match,omitempty" yaml:"payload_match,omitempty"`
	Cron         string         `json:"cron,omitempty" yaml:"cron,omitempty"`
	Timezone     string         `json:"timezone,omitempty" yaml:"timezone,omitempty"`
	Interval     string         `json:"interval,omitempty" yaml:"interval,omitempty"`
	Threshold    *Threshold     `json:"threshold,omitempty" yaml:"threshold,omitempty"`
	Any          []Spec         `json:"any,omitempty" yaml:"any,omitempty"`
	All          []Spec         `json:"all,omitempty" yaml:"all,omitempty"`
}

type Threshold struct {
	Metric string  `json:"metric" yaml:"metric"`
	Op     string  `json:"op" yaml:"op"`
	Value  float64 `json:"value" yaml:"value"`
	Window string  `json:"window,omitempty" yaml:"window,omitempty"`
}

type Input struct {
	Events          []schema.Event
	Metrics         metric.Registry
	MetricContext   metric.Context
	LastTriggeredAt time.Time
}

type Decision struct {
	Matched bool
	Reason  string
	Events  []schema.Event
	Metrics map[string]float64
}

func Evaluate(ctx context.Context, spec Spec, input Input) (Decision, error) {
	if input.Metrics == nil {
		input.Metrics = metric.DefaultRegistry()
	}
	return evaluate(ctx, spec, input)
}

func evaluate(ctx context.Context, spec Spec, input Input) (Decision, error) {
	switch {
	case spec.Event != "":
		return evaluateEvent(spec, input), nil
	case spec.Cron != "":
		return evaluateCron(spec, input)
	case spec.Interval != "":
		return evaluateInterval(spec, input)
	case spec.Threshold != nil:
		return evaluateThreshold(ctx, *spec.Threshold, input)
	case len(spec.Any) > 0:
		return evaluateAny(ctx, spec.Any, input)
	case len(spec.All) > 0:
		return evaluateAll(ctx, spec.All, input)
	default:
		return Decision{}, fmt.Errorf("trigger has no condition")
	}
}

func evaluateEvent(spec Spec, input Input) Decision {
	var matched []schema.Event
	for _, event := range input.Events {
		if event.Type != spec.Event || !payloadMatches(event.Payload, spec.PayloadMatch) {
			continue
		}
		matched = append(matched, event)
	}
	return Decision{Matched: len(matched) > 0, Reason: "event:" + spec.Event, Events: matched}
}

func evaluateCron(spec Spec, input Input) (Decision, error) {
	now := input.MetricContext.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if spec.Timezone != "" {
		loc, err := time.LoadLocation(spec.Timezone)
		if err != nil {
			return Decision{}, err
		}
		now = now.In(loc)
	}
	matched, err := CronMatches(spec.Cron, now)
	if err != nil {
		return Decision{}, err
	}
	return Decision{Matched: matched, Reason: "cron:" + spec.Cron}, nil
}

func evaluateInterval(spec Spec, input Input) (Decision, error) {
	dur, err := time.ParseDuration(spec.Interval)
	if err != nil {
		return Decision{}, err
	}
	now := input.MetricContext.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if input.LastTriggeredAt.IsZero() {
		return Decision{Matched: true, Reason: "interval:first:" + spec.Interval}, nil
	}
	return Decision{Matched: now.Sub(input.LastTriggeredAt) >= dur, Reason: "interval:" + spec.Interval}, nil
}

func evaluateThreshold(ctx context.Context, threshold Threshold, input Input) (Decision, error) {
	collector, ok := input.Metrics[threshold.Metric]
	if !ok {
		return Decision{}, fmt.Errorf("unknown metric %q", threshold.Metric)
	}
	value, err := collector.Collect(ctx, input.MetricContext)
	if err != nil {
		return Decision{}, err
	}
	return Decision{
		Matched: compare(value, threshold.Op, threshold.Value),
		Reason:  "threshold:" + threshold.Metric,
		Metrics: map[string]float64{threshold.Metric: value},
	}, nil
}

func evaluateAny(ctx context.Context, specs []Spec, input Input) (Decision, error) {
	var decision Decision
	decision.Reason = "any"
	decision.Metrics = map[string]float64{}
	for _, spec := range specs {
		child, err := evaluate(ctx, spec, input)
		if err != nil {
			return Decision{}, err
		}
		if child.Matched {
			decision.Matched = true
		}
		decision.Events = append(decision.Events, child.Events...)
		for key, value := range child.Metrics {
			decision.Metrics[key] = value
		}
	}
	if len(decision.Metrics) == 0 {
		decision.Metrics = nil
	}
	return decision, nil
}

func evaluateAll(ctx context.Context, specs []Spec, input Input) (Decision, error) {
	decision := Decision{Matched: true, Reason: "all", Metrics: map[string]float64{}}
	for _, spec := range specs {
		child, err := evaluate(ctx, spec, input)
		if err != nil {
			return Decision{}, err
		}
		if !child.Matched {
			decision.Matched = false
		}
		decision.Events = append(decision.Events, child.Events...)
		for key, value := range child.Metrics {
			decision.Metrics[key] = value
		}
	}
	if len(decision.Metrics) == 0 {
		decision.Metrics = nil
	}
	return decision, nil
}

func payloadMatches(payload map[string]any, expected map[string]any) bool {
	for key, want := range expected {
		got, ok := payload[key]
		if !ok || fmt.Sprint(got) != fmt.Sprint(want) {
			return false
		}
	}
	return true
}

func compare(got float64, op string, want float64) bool {
	switch op {
	case ">":
		return got > want
	case ">=":
		return got >= want
	case "<":
		return got < want
	case "<=":
		return got <= want
	case "==":
		return got == want
	case "!=":
		return got != want
	default:
		return false
	}
}

func CronMatches(expr string, now time.Time) (bool, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false, fmt.Errorf("cron %q must have 5 fields", expr)
	}
	values := []int{now.Minute(), now.Hour(), now.Day(), int(now.Month()), int(now.Weekday())}
	for index, field := range fields {
		matched, err := cronFieldMatches(field, values[index])
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func cronFieldMatches(field string, value int) (bool, error) {
	for _, part := range strings.Split(field, ",") {
		matched, err := cronPartMatches(part, value)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

// cronPartMatches reports whether value satisfies one comma-separated cron field
// part. Supported grammar: "*", "*/step", "n", "lo-hi", "lo-hi/step", "n/step".
func cronPartMatches(part string, value int) (bool, error) {
	base := part
	step := 0
	if i := strings.Index(part, "/"); i >= 0 {
		base = part[:i]
		s, err := strconv.Atoi(part[i+1:])
		if err != nil || s <= 0 {
			return false, fmt.Errorf("invalid cron step %q", part)
		}
		step = s
	}
	if base == "*" {
		if step == 0 {
			return true, nil
		}
		return value%step == 0, nil
	}
	if lo, hi, ok, err := cronRange(base); err != nil {
		return false, err
	} else if ok {
		if value < lo || value > hi {
			return false, nil
		}
		if step == 0 {
			return true, nil
		}
		return (value-lo)%step == 0, nil
	}
	n, err := strconv.Atoi(base)
	if err != nil {
		return false, fmt.Errorf("invalid cron field %q", part)
	}
	if step == 0 {
		return value == n, nil
	}
	return value >= n && (value-n)%step == 0, nil
}

// cronRange parses a "lo-hi" cron range. ok is false when s is not a range.
func cronRange(s string) (int, int, bool, error) {
	i := strings.Index(s, "-")
	if i < 0 {
		return 0, 0, false, nil
	}
	lo, err1 := strconv.Atoi(s[:i])
	hi, err2 := strconv.Atoi(s[i+1:])
	if err1 != nil || err2 != nil || lo > hi {
		return 0, 0, false, fmt.Errorf("invalid cron range %q", s)
	}
	return lo, hi, true, nil
}
