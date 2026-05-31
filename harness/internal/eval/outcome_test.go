package eval

import (
	"errors"
	"testing"
)

func TestDeriveOutcome(t *testing.T) {
	tests := []struct {
		name  string
		input OutcomeInput
		want  Outcome
	}{
		{
			name: "all assertions pass",
			input: OutcomeInput{Assertions: []AssertionResult{
				{Name: "first", Passed: true},
				{Name: "second", Passed: true},
			}},
			want: OutcomePass,
		},
		{
			name: "partial assertion pass is weak",
			input: OutcomeInput{Assertions: []AssertionResult{
				{Name: "first", Passed: true},
				{Name: "second", Passed: false},
			}},
			want: OutcomeWeak,
		},
		{
			name: "all assertions fail",
			input: OutcomeInput{Assertions: []AssertionResult{
				{Name: "first", Passed: false},
				{Name: "second", Passed: false},
			}},
			want: OutcomeFail,
		},
		{
			name:  "no assertions means noop",
			input: OutcomeInput{},
			want:  OutcomeNoop,
		},
		{
			name:  "assertion runtime error is invalid",
			input: OutcomeInput{AssertionErr: errors.New("protocol error")},
			want:  OutcomeInvalid,
		},
		{
			name: "explicit human review need is proposal",
			input: OutcomeInput{
				ProposalRequired: true,
				Assertions:       []AssertionResult{{Name: "needs review", Passed: true}},
			},
			want: OutcomeProposal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveOutcome(tc.input); got != tc.want {
				t.Fatalf("DeriveOutcome() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestScenarioAreaUsesMetadataBeforeIDFallback(t *testing.T) {
	tests := []struct {
		name      string
		scenario  Scenario
		wantArea  string
		wantRoute string
	}{
		{
			name:      "explicit docs area",
			scenario:  Scenario{ID: "memory-looking-doc-case", Area: "docs", Loops: []string{"memory"}},
			wantArea:  "docs",
			wantRoute: "docs",
		},
		{
			name:      "loop metadata",
			scenario:  Scenario{ID: "custom-skill-case", Loops: []string{"eval", "skill"}},
			wantArea:  "skill",
			wantRoute: "skill",
		},
		{
			name:      "id fallback",
			scenario:  Scenario{ID: "memory-focused-recall"},
			wantArea:  "memory",
			wantRoute: "memory",
		},
		{
			name:      "ops alias",
			scenario:  Scenario{ID: "ops-host-projection", Area: "ops"},
			wantArea:  "projection",
			wantRoute: "projection",
		},
		{
			name:      "unknown fallback",
			scenario:  Scenario{ID: "custom"},
			wantArea:  "eval",
			wantRoute: "eval",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			area := ScenarioArea(tc.scenario)
			if area != tc.wantArea {
				t.Fatalf("ScenarioArea() = %s, want %s", area, tc.wantArea)
			}
			if route := ProposalRouteForArea(area); route != tc.wantRoute {
				t.Fatalf("ProposalRouteForArea() = %s, want %s", route, tc.wantRoute)
			}
		})
	}
}

func TestScenarioAreaRoutesByLoop(t *testing.T) {
	if area := ScenarioArea(Scenario{ID: "memory-no-pollution", Loops: []string{"memory"}}); area != "memory" {
		t.Fatalf("expected memory area, got %q", area)
	}
}
