package trigger

import (
	"context"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/metric"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func TestEvaluateEventTriggerWithPayloadMatch(t *testing.T) {
	decision, err := Evaluate(context.Background(), Spec{
		Event:        "memory.hot_write_observed",
		PayloadMatch: map[string]any{"severity": "high"},
	}, Input{Events: []schema.Event{{
		ID:      "evt_1",
		Type:    "memory.hot_write_observed",
		Payload: map[string]any{"severity": "high"},
	}}})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.Matched || len(decision.Events) != 1 {
		t.Fatalf("expected matched event, got %#v", decision)
	}
}

func TestEvaluateCronTrigger(t *testing.T) {
	decision, err := Evaluate(context.Background(), Spec{Cron: "0 3 * * *"}, Input{
		MetricContext: metric.Context{Now: time.Date(2026, 5, 28, 3, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.Matched {
		t.Fatalf("expected cron to match")
	}
}

// Regression for cron range/step support: valid POSIX field syntax must match at
// runtime instead of erroring and aborting the daemon tick.
func TestCronFieldMatchesRangeAndStep(t *testing.T) {
	cases := []struct {
		field string
		value int
		want  bool
	}{
		{"1-5", 3, true}, {"1-5", 1, true}, {"1-5", 5, true},
		{"1-5", 0, false}, {"1-5", 6, false},
		{"*/15", 30, true}, {"*/15", 31, false},
		{"0-30/10", 20, true}, {"0-30/10", 25, false},
		{"5", 5, true}, {"5", 6, false},
		{"1,3,5", 3, true}, {"1,3,5", 2, false},
		{"*", 17, true},
	}
	for _, c := range cases {
		got, err := cronFieldMatches(c.field, c.value)
		if err != nil {
			t.Fatalf("cronFieldMatches(%q,%d) error: %v", c.field, c.value, err)
		}
		if got != c.want {
			t.Errorf("cronFieldMatches(%q,%d)=%v want %v", c.field, c.value, got, c.want)
		}
	}
	if _, err := cronFieldMatches("abc", 1); err == nil {
		t.Fatalf("expected error for unparseable cron field")
	}
}

func TestEvaluateIntervalTrigger(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	decision, err := Evaluate(context.Background(), Spec{Interval: "6h"}, Input{
		MetricContext:   metric.Context{Now: now},
		LastTriggeredAt: now.Add(-7 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.Matched {
		t.Fatalf("expected interval to match")
	}
}

func TestEvaluateThresholdAndComposite(t *testing.T) {
	registry := metric.Registry{
		"memory.lines": metric.CollectorFunc(func(context.Context, metric.Context) (float64, error) {
			return 250, nil
		}),
	}
	decision, err := Evaluate(context.Background(), Spec{Any: []Spec{
		{Event: "memory.hot_write_observed"},
		{Threshold: &Threshold{Metric: "memory.lines", Op: ">", Value: 200}},
	}}, Input{Metrics: registry})
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.Matched || decision.Metrics["memory.lines"] != 250 {
		t.Fatalf("expected threshold composite match, got %#v", decision)
	}
}
