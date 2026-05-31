package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
)

func TestProposalCommandSmoke(t *testing.T) {
	root := t.TempDir()
	restoreProposalFlags(t)
	proposalRoot = root

	createProposalFixture(t, "prop-cli-main")
	createCmd, createOutput := testCommand()
	if err := runProposalCreate(createCmd, nil); err != nil {
		t.Fatalf("runProposalCreate returned error: %v", err)
	}
	if !strings.Contains(createOutput.String(), "created proposal prop-cli-main") {
		t.Fatalf("unexpected create output: %s", createOutput.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "proposals", "draft", "prop-cli-main", "proposal.json")); err != nil {
		t.Fatalf("expected proposal file: %v", err)
	}
	proposalData, err := os.ReadFile(filepath.Join(root, ".mnemon", "harness", "proposals", "draft", "prop-cli-main", "proposal.json"))
	if err != nil {
		t.Fatalf("read proposal file: %v", err)
	}
	if !strings.Contains(string(proposalData), `"scope"`) || !strings.Contains(string(proposalData), `"loop": "memory"`) {
		t.Fatalf("proposal missing default memory scope:\n%s", string(proposalData))
	}

	clearProposalContentFlags()
	listCmd, listOutput := testCommand()
	if err := runProposalList(listCmd, nil); err != nil {
		t.Fatalf("runProposalList returned error: %v", err)
	}
	if !strings.Contains(listOutput.String(), "prop-cli-main") {
		t.Fatalf("unexpected list output: %s", listOutput.String())
	}

	proposalID = "prop-cli-main"
	showCmd, showOutput := testCommand()
	if err := runProposalShow(showCmd, nil); err != nil {
		t.Fatalf("runProposalShow returned error: %v", err)
	}
	if !strings.Contains(showOutput.String(), "proposal prop-cli-main: draft") {
		t.Fatalf("unexpected show output: %s", showOutput.String())
	}

	transitionWithUpdate(t, "prop-cli-main", "open")
	transitionWithUpdate(t, "prop-cli-main", "in_review")
	approveCmd, approveOutput := testCommand()
	if err := runProposalTransition(approveCmd, "approved"); err != nil {
		t.Fatalf("approve transition returned error: %v", err)
	}
	if !strings.Contains(approveOutput.String(), "approved") {
		t.Fatalf("unexpected approve output: %s", approveOutput.String())
	}
	err = runProposalApply(mustTestCommand(t), nil)
	if !errors.Is(err, app.ErrProposalApplyNotImplemented) {
		t.Fatalf("expected apply not implemented error, got %v", err)
	}
	auditRecords, err := os.ReadDir(filepath.Join(root, ".mnemon", "harness", "audit", "records"))
	if err != nil {
		t.Fatalf("expected proposal apply boundary audit record: %v", err)
	}
	if len(auditRecords) != 1 {
		t.Fatalf("expected 1 proposal apply boundary audit record, got %d", len(auditRecords))
	}

	createProposalFixture(t, "prop-cli-changes")
	if err := runProposalCreate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("create request-changes fixture: %v", err)
	}
	transitionWithUpdate(t, "prop-cli-changes", "open")
	proposalID = "prop-cli-changes"
	if err := runProposalTransition(mustTestCommand(t), "request_changes"); err != nil {
		t.Fatalf("request-changes transition returned error: %v", err)
	}

	createProposalFixture(t, "prop-cli-block")
	if err := runProposalCreate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("create block fixture: %v", err)
	}
	transitionWithUpdate(t, "prop-cli-block", "open")
	proposalID = "prop-cli-block"
	if err := runProposalTransition(mustTestCommand(t), "blocked"); err != nil {
		t.Fatalf("block transition returned error: %v", err)
	}

	createProposalFixture(t, "prop-cli-reject")
	if err := runProposalCreate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("create reject fixture: %v", err)
	}
	transitionWithUpdate(t, "prop-cli-reject", "open")
	transitionWithUpdate(t, "prop-cli-reject", "in_review")
	proposalID = "prop-cli-reject"
	if err := runProposalTransition(mustTestCommand(t), "rejected"); err != nil {
		t.Fatalf("reject transition returned error: %v", err)
	}

	createProposalFixture(t, "prop-cli-new")
	if err := runProposalCreate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("create superseding fixture: %v", err)
	}
	createProposalFixture(t, "prop-cli-old")
	if err := runProposalCreate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("create superseded fixture: %v", err)
	}
	transitionWithUpdate(t, "prop-cli-old", "open")
	proposalID = "prop-cli-old"
	proposalSupersededBy = "prop-cli-new"
	if err := runProposalSupersede(mustTestCommand(t), nil); err != nil {
		t.Fatalf("runProposalSupersede returned error: %v", err)
	}

	createProposalFixture(t, "prop-cli-withdraw")
	if err := runProposalCreate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("create withdraw fixture: %v", err)
	}
	proposalID = "prop-cli-withdraw"
	if err := runProposalTransition(mustTestCommand(t), "withdrawn"); err != nil {
		t.Fatalf("withdraw transition returned error: %v", err)
	}

	createProposalFixture(t, "prop-cli-expire")
	if err := runProposalCreate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("create expire fixture: %v", err)
	}
	proposalID = "prop-cli-expire"
	if err := runProposalTransition(mustTestCommand(t), "expired"); err != nil {
		t.Fatalf("expire transition returned error: %v", err)
	}

	types := proposalEventTypes(t, root)
	for _, want := range []string{
		"proposal.created",
		"proposal.opened",
		"proposal.in_review",
		"proposal.approved",
		"proposal.request_changes",
		"proposal.blocked",
		"proposal.rejected",
		"proposal.superseded",
		"proposal.withdrawn",
		"proposal.expired",
		"audit.recorded",
	} {
		if !types[want] {
			t.Fatalf("missing event type %s", want)
		}
	}
}

func TestProposalCreateRecordsExplicitScope(t *testing.T) {
	root := t.TempDir()
	restoreProposalFlags(t)
	proposalRoot = root
	createProposalFixture(t, "prop-cli-scope")
	proposalScopeStore = "work"
	proposalScopeHost = "codex"
	proposalScopeLoop = "memory"
	proposalScopeProfileRef = "profile:personal/default"

	if err := runProposalCreate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("runProposalCreate returned error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".mnemon", "harness", "proposals", "draft", "prop-cli-scope", "proposal.json"))
	if err != nil {
		t.Fatalf("read proposal: %v", err)
	}
	for _, want := range []string{
		`"store": "work"`,
		`"host": "codex"`,
		`"loop": "memory"`,
		`"profile_ref": "profile:personal/default"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("expected %s in proposal:\n%s", want, string(data))
		}
	}
	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(allEvents) != 1 || allEvents[0].Scope["profile_ref"] != "profile:personal/default" {
		t.Fatalf("expected scoped proposal.created event, got %#v", allEvents)
	}
}

func TestProposalApplyEvalPromotesAssetAndAudits(t *testing.T) {
	root := t.TempDir()
	writeEvalRunFixture(t, root)
	id := createEvalCommandApprovedProposal(t, root, "eval-apply-cli")
	restoreProposalFlags(t)
	proposalRoot = root
	proposalID = id

	cmd, output := testCommand()
	if err := runProposalApply(cmd, nil); err != nil {
		t.Fatalf("runProposalApply returned error: %v", err)
	}
	for _, want := range []string{
		"proposal eval-apply-cli applied",
		"route: eval",
		"eval asset: suite default",
		"event:",
		"audit:",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
	appliedPath := filepath.Join(root, ".mnemon", "harness", "proposals", "applied", id, "proposal.json")
	data, err := os.ReadFile(appliedPath)
	if err != nil {
		t.Fatalf("read applied proposal: %v", err)
	}
	if !strings.Contains(string(data), `"status": "applied"`) || !strings.Contains(string(data), `"audit_refs"`) {
		t.Fatalf("applied proposal missing status/audit refs:\n%s", string(data))
	}

	types := proposalEventTypes(t, root)
	for _, want := range []string{
		"eval.asset_promoted",
		"audit.recorded",
		"proposal.applied",
	} {
		if !types[want] {
			t.Fatalf("missing event type %s", want)
		}
	}
	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	for _, event := range allEvents {
		if event.Type == "eval.asset_promoted" || event.Type == "audit.recorded" {
			if event.Scope["binding_scope"] != "project" || event.Scope["loop"] != "eval" {
				t.Fatalf("expected project eval scope on %s: %#v", event.Type, event.Scope)
			}
		}
	}
}

func TestProposalApplyMemoryProfileEntryAddsProfileAndAudits(t *testing.T) {
	root := t.TempDir()
	restoreProposalFlags(t)
	proposalRoot = root
	proposalID = "memory-profile-apply-cli"
	proposalRoute = "memory"
	proposalRisk = "medium"
	proposalTitle = "Record profile work style"
	proposalSummary = "Approve a durable profile entry for future host agents."
	proposalChangeSummary = "Add one evidence-backed profile entry."
	proposalTargets = []string{"profile_entry=profile:personal/personal-default"}
	proposalOperations = []string{`profile.entry.add=profile:personal/personal-default=Record focused commit preference={"entry_id":"focused-commits","entry_type":"work_style","summary":"Prefer focused harness commits","content":"Keep harness changes staged and avoid stable mnemon release paths.","project_to":["codex/memory"]}`}
	proposalEvidence = []string{"manual=goal:E3=User approved profile update"}
	proposalValidationSummary = "Show filtered profile entry."
	proposalScopeProfileRef = "profile:personal/personal-default"

	if err := runProposalCreate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("runProposalCreate returned error: %v", err)
	}
	transitionWithUpdate(t, "memory-profile-apply-cli", "open")
	transitionWithUpdate(t, "memory-profile-apply-cli", "in_review")
	proposalID = "memory-profile-apply-cli"
	if err := runProposalTransition(mustTestCommand(t), "approved"); err != nil {
		t.Fatalf("approve transition returned error: %v", err)
	}
	cmd, output := testCommand()
	if err := runProposalApply(cmd, nil); err != nil {
		t.Fatalf("runProposalApply returned error: %v", err)
	}
	for _, want := range []string{
		"proposal memory-profile-apply-cli applied",
		"route: memory",
		"profile entry: profile:personal/personal-default focused-commits",
		"audit:",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
	profileData, err := os.ReadFile(filepath.Join(root, ".mnemon", "harness", "profiles", "personal-default", "profile.json"))
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	for _, want := range []string{
		`"id": "focused-commits"`,
		`"type": "work_style"`,
		`"ref": "goal:E3"`,
		`"host": "codex"`,
		`"loop": "memory"`,
	} {
		if !strings.Contains(string(profileData), want) {
			t.Fatalf("expected %s in profile:\n%s", want, string(profileData))
		}
	}
	appliedPath := filepath.Join(root, ".mnemon", "harness", "proposals", "applied", "memory-profile-apply-cli", "proposal.json")
	appliedData, err := os.ReadFile(appliedPath)
	if err != nil {
		t.Fatalf("read applied proposal: %v", err)
	}
	if !strings.Contains(string(appliedData), `"audit_refs"`) {
		t.Fatalf("applied proposal missing audit refs:\n%s", string(appliedData))
	}
	types := proposalEventTypes(t, root)
	for _, want := range []string{
		"profile.entry_recorded",
		"audit.recorded",
		"proposal.applied",
	} {
		if !types[want] {
			t.Fatalf("missing event type %s", want)
		}
	}
	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	for _, event := range allEvents {
		if event.Type == "profile.entry_recorded" || event.Type == "audit.recorded" {
			if event.Scope["profile_ref"] != "profile:personal/personal-default" {
				t.Fatalf("expected profile_ref scope on %s: %#v", event.Type, event.Scope)
			}
		}
	}
}

func createProposalFixture(t *testing.T, id string) {
	t.Helper()
	clearProposalContentFlags()
	proposalID = id
	proposalRoute = "memory"
	proposalRisk = "medium"
	proposalTitle = "Review memory lifecycle change"
	proposalSummary = "Review a proposed memory lifecycle change."
	proposalChangeSummary = "Write durable project preference memory."
	proposalTargets = []string{"memory=mnemon://memory/project/preferences"}
	proposalValidationSummary = "Run memory recall validation."
}

func transitionWithUpdate(t *testing.T, id, status string) {
	t.Helper()
	clearProposalContentFlags()
	proposalID = id
	proposalStatus = status
	if err := runProposalUpdate(mustTestCommand(t), nil); err != nil {
		t.Fatalf("transition %s to %s: %v", id, status, err)
	}
	proposalStatus = ""
}

func proposalEventTypes(t *testing.T, root string) map[string]bool {
	t.Helper()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	types := map[string]bool{}
	for _, event := range events {
		types[event.Type] = true
	}
	return types
}

func restoreProposalFlags(t *testing.T) {
	t.Helper()
	oldRoot := proposalRoot
	oldID := proposalID
	oldRoute := proposalRoute
	oldRisk := proposalRisk
	oldTitle := proposalTitle
	oldSummary := proposalSummary
	oldChangeSummary := proposalChangeSummary
	oldTargets := proposalTargets
	oldOperations := proposalOperations
	oldEvidence := proposalEvidence
	oldValidationSummary := proposalValidationSummary
	oldValidationCommands := proposalValidationCommands
	oldValidationChecks := proposalValidationChecks
	oldReviewRequired := proposalReviewRequired
	oldReviewScope := proposalReviewScope
	oldRequiredReviews := proposalRequiredReviews
	oldReviewers := proposalReviewers
	oldReviewNotes := proposalReviewNotes
	oldScopeStore := proposalScopeStore
	oldScopeHost := proposalScopeHost
	oldScopeLoop := proposalScopeLoop
	oldScopeProfileRef := proposalScopeProfileRef
	oldStatus := proposalStatus
	oldListStatuses := proposalListStatuses
	oldSupersededBy := proposalSupersededBy
	oldFormat := proposalFormat
	t.Cleanup(func() {
		proposalRoot = oldRoot
		proposalID = oldID
		proposalRoute = oldRoute
		proposalRisk = oldRisk
		proposalTitle = oldTitle
		proposalSummary = oldSummary
		proposalChangeSummary = oldChangeSummary
		proposalTargets = oldTargets
		proposalOperations = oldOperations
		proposalEvidence = oldEvidence
		proposalValidationSummary = oldValidationSummary
		proposalValidationCommands = oldValidationCommands
		proposalValidationChecks = oldValidationChecks
		proposalReviewRequired = oldReviewRequired
		proposalReviewScope = oldReviewScope
		proposalRequiredReviews = oldRequiredReviews
		proposalReviewers = oldReviewers
		proposalReviewNotes = oldReviewNotes
		proposalScopeStore = oldScopeStore
		proposalScopeHost = oldScopeHost
		proposalScopeLoop = oldScopeLoop
		proposalScopeProfileRef = oldScopeProfileRef
		proposalStatus = oldStatus
		proposalListStatuses = oldListStatuses
		proposalSupersededBy = oldSupersededBy
		proposalFormat = oldFormat
	})
	clearProposalContentFlags()
	proposalRoot = "."
}

func clearProposalContentFlags() {
	proposalID = ""
	proposalRoute = "memory"
	proposalRisk = "medium"
	proposalTitle = ""
	proposalSummary = ""
	proposalChangeSummary = ""
	proposalTargets = nil
	proposalOperations = nil
	proposalEvidence = nil
	proposalValidationSummary = ""
	proposalValidationCommands = nil
	proposalValidationChecks = nil
	proposalReviewRequired = false
	proposalReviewScope = ""
	proposalRequiredReviews = 0
	proposalReviewers = nil
	proposalReviewNotes = ""
	proposalScopeStore = ""
	proposalScopeHost = ""
	proposalScopeLoop = ""
	proposalScopeProfileRef = ""
	proposalStatus = ""
	proposalListStatuses = nil
	proposalSupersededBy = ""
	proposalFormat = "text"
}
