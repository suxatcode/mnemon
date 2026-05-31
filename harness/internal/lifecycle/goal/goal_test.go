package goal

import "testing"

func TestValidateGoalStatus(t *testing.T) {
	for _, status := range []Status{
		StatusDraft,
		StatusPlanned,
		StatusActive,
		StatusVerifying,
		StatusComplete,
		StatusBlocked,
		StatusPaused,
	} {
		if err := ValidateStatus(status); err != nil {
			t.Fatalf("ValidateStatus(%q) returned error: %v", status, err)
		}
	}
	if err := ValidateStatus("unknown"); err == nil {
		t.Fatal("expected invalid status error")
	}
}

func TestCompletionReadyRequiresPassingReportAndAcceptedEvidence(t *testing.T) {
	evidence := []GoalEvidence{{
		ID:     "evidence-1",
		Status: "accepted",
	}}
	report := &GoalReport{
		Status: "pass",
		VerificationGate: VerificationGate{
			Passed: true,
		},
		EvidenceRefs: []string{"evidence-1"},
	}
	if !CompletionReady(report, evidence) {
		t.Fatal("expected completion to be ready")
	}
	report.EvidenceRefs = []string{"missing"}
	if CompletionReady(report, evidence) {
		t.Fatal("expected missing evidence ref to block completion")
	}
	report.EvidenceRefs = []string{"evidence-1"}
	report.VerificationGate.Passed = false
	if CompletionReady(report, evidence) {
		t.Fatal("expected failed gate to block completion")
	}
}

func TestValidateTransition(t *testing.T) {
	valid := []struct {
		from Status
		to   Status
	}{
		{StatusDraft, StatusPlanned},
		{StatusDraft, StatusPaused},
		{StatusPlanned, StatusVerifying},
		{StatusActive, StatusPaused},
		{StatusVerifying, StatusVerifying},
		{StatusVerifying, StatusComplete},
		{StatusPaused, StatusActive},
	}
	for _, tc := range valid {
		if err := ValidateTransition(tc.from, tc.to); err != nil {
			t.Fatalf("ValidateTransition(%s, %s) returned error: %v", tc.from, tc.to, err)
		}
	}
	invalid := []struct {
		from Status
		to   Status
	}{
		{StatusDraft, StatusVerifying},
		{StatusActive, StatusComplete},
		{StatusPaused, StatusComplete},
		{StatusComplete, StatusBlocked},
		{StatusBlocked, StatusActive},
	}
	for _, tc := range invalid {
		if err := ValidateTransition(tc.from, tc.to); err == nil {
			t.Fatalf("ValidateTransition(%s, %s) succeeded", tc.from, tc.to)
		}
	}
}

const ts = "2026-05-29T00:00:00Z"

func TestValidateGoal(t *testing.T) {
	valid := Goal{
		SchemaVersion: SchemaVersion, Kind: "Goal", ID: "goal-1",
		Objective: "ship v0.3", Status: StatusActive,
		CreatedAt: ts, UpdatedAt: ts,
	}
	if err := ValidateGoal(valid); err != nil {
		t.Fatalf("valid goal rejected: %v", err)
	}
	for name, mut := range map[string]func(*Goal){
		"bad schema_version": func(g *Goal) { g.SchemaVersion = "wrong" },
		"bad kind":           func(g *Goal) { g.Kind = "Nope" },
		"empty id":           func(g *Goal) { g.ID = "" },
		"empty objective":    func(g *Goal) { g.Objective = "" },
		"bad status":         func(g *Goal) { g.Status = "bogus" },
		"bad created_at":     func(g *Goal) { g.CreatedAt = "not-a-date" },
		"negative evidence":  func(g *Goal) { g.EvidenceCount = -1 },
	} {
		bad := valid
		mut(&bad)
		if err := ValidateGoal(bad); err == nil {
			t.Errorf("expected %s to fail validation", name)
		}
	}
}

func TestValidatePlan(t *testing.T) {
	valid := GoalPlan{
		SchemaVersion: PlanSchemaVersion, Kind: "GoalPlan", GoalID: "goal-1",
		Summary: "do the thing", CreatedAt: ts, UpdatedAt: ts,
	}
	if err := ValidatePlan(valid); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	for name, mut := range map[string]func(*GoalPlan){
		"bad kind":            func(p *GoalPlan) { p.Kind = "Nope" },
		"empty goal_id":       func(p *GoalPlan) { p.GoalID = "" },
		"no summary or steps": func(p *GoalPlan) { p.Summary = "" },
		"empty step":          func(p *GoalPlan) { p.Summary = ""; p.Steps = []string{" "} },
		"bad created_at":      func(p *GoalPlan) { p.CreatedAt = "nope" },
	} {
		bad := valid
		mut(&bad)
		if err := ValidatePlan(bad); err == nil {
			t.Errorf("expected %s to fail validation", name)
		}
	}
}

func TestValidateEvidence(t *testing.T) {
	valid := GoalEvidence{
		SchemaVersion: EvidenceSchemaVersion, Kind: "GoalEvidence", ID: "ev-1",
		GoalID: "goal-1", Type: "manual", Status: "accepted",
		Summary: "did x", RecordedAt: ts,
	}
	if err := ValidateEvidence(valid); err != nil {
		t.Fatalf("valid evidence rejected: %v", err)
	}
	for name, mut := range map[string]func(*GoalEvidence){
		"bad type":        func(e *GoalEvidence) { e.Type = "nope" },
		"bad status":      func(e *GoalEvidence) { e.Status = "nope" },
		"empty goal_id":   func(e *GoalEvidence) { e.GoalID = "" },
		"empty summary":   func(e *GoalEvidence) { e.Summary = "" },
		"bad recorded_at": func(e *GoalEvidence) { e.RecordedAt = "nope" },
	} {
		bad := valid
		mut(&bad)
		if err := ValidateEvidence(bad); err == nil {
			t.Errorf("expected %s to fail validation", name)
		}
	}
}

func TestValidateReport(t *testing.T) {
	valid := GoalReport{
		SchemaVersion: ReportSchemaVersion, Kind: "GoalReport", ID: "rep-1",
		GoalID: "goal-1", Status: "pass", Summary: "ok", GeneratedAt: ts,
		VerificationGate: VerificationGate{Name: "gate", CheckedAt: ts, Passed: true},
		EvidenceRefs:     []string{"ev-1"},
	}
	if err := ValidateReport(valid); err != nil {
		t.Fatalf("valid report rejected: %v", err)
	}
	for name, mut := range map[string]func(*GoalReport){
		"bad status":            func(r *GoalReport) { r.Status = "nope" },
		"empty summary":         func(r *GoalReport) { r.Summary = "" },
		"missing gate name":     func(r *GoalReport) { r.VerificationGate.Name = "" },
		"pass without gate":     func(r *GoalReport) { r.VerificationGate.Passed = false },
		"pass without evidence": func(r *GoalReport) { r.EvidenceRefs = nil },
	} {
		bad := valid
		mut(&bad)
		if err := ValidateReport(bad); err == nil {
			t.Errorf("expected %s to fail validation", name)
		}
	}
}

func TestValidateHostGoalLink(t *testing.T) {
	valid := HostGoalLink{
		SchemaVersion: HostLinkSchemaVersion, Kind: "HostGoalLink", ID: "link-1",
		GoalID: "goal-1", Host: "codex", ThreadID: "thread-1",
		Objective: "ship", LinkedAt: ts,
	}
	if err := ValidateHostGoalLink(valid); err != nil {
		t.Fatalf("valid host link rejected: %v", err)
	}
	for name, mut := range map[string]func(*HostGoalLink){
		"empty host":             func(l *HostGoalLink) { l.Host = "" },
		"no thread or host goal": func(l *HostGoalLink) { l.ThreadID = ""; l.HostGoalID = "" },
		"empty objective":        func(l *HostGoalLink) { l.Objective = "" },
		"bad linked_at":          func(l *HostGoalLink) { l.LinkedAt = "nope" },
	} {
		bad := valid
		mut(&bad)
		if err := ValidateHostGoalLink(bad); err == nil {
			t.Errorf("expected %s to fail validation", name)
		}
	}
}
