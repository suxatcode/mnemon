package read

import "strings"

// ReviewClass is a DETERMINISTIC, code-computed triage hint for a proposal — an
// advisory signal of blast-radius / review effort, shown as a badge to help a
// reviewer scan a queue. It is NEVER an auto-apply decision and NEVER a model
// verdict: the human reviews and presses apply on every proposal. (When policy-
// gated auto-apply arrives in a future cycle it will be a separate, governed,
// code-level eligibility rule — not this advisory badge.)
type ReviewClass struct {
	Safe   bool   // narrow, reversible, low blast-radius — quick to review
	Label  string // "safe" | "review"
	Reason string // why, in one phrase
}

// ClassifyProposal returns the advisory triage class for a proposal, computed
// purely from its route, operation, and risk — no model, no I/O. High-blast or
// hard-to-reverse changes are always "review"; narrow, reversible edits are
// "safe" (advisory only).
func ClassifyProposal(p Proposal) ReviewClass {
	risk := strings.ToLower(strings.TrimSpace(p.Risk))
	op := ""
	if len(p.Change.Operations) > 0 {
		op = strings.ToLower(p.Change.Operations[0].Type)
	}

	// Durable, hard-to-reverse routes always warrant careful review.
	switch p.Route {
	case "memory", "profile", "skill", "guide":
		return ReviewClass{Safe: false, Label: "review", Reason: "durable " + p.Route + " change — hard to reverse"}
	}

	if p.Route == "coordination" {
		switch {
		case containsAny(op, "merge", "reassign", "join", "conflict"):
			return ReviewClass{Safe: false, Label: "review", Reason: "cross-agent blast radius"}
		case containsAny(op, "unlink", "member_removed", "link", "member", "group"):
			return ReviewClass{Safe: true, Label: "safe", Reason: "narrow, reversible coordination edit"}
		}
	}

	if risk == "high" || risk == "critical" {
		return ReviewClass{Safe: false, Label: "review", Reason: "risk=" + risk}
	}
	if risk == "low" {
		return ReviewClass{Safe: true, Label: "safe", Reason: "low risk"}
	}
	return ReviewClass{Safe: false, Label: "review", Reason: "review before apply"}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
