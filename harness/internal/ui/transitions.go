package ui

// legalTransitions mirrors proposal.transitions (the state machine in
// harness/internal/lifecycle/proposal). The UI uses it only to offer / disable
// actions; the facade re-validates every transition, so this table is advisory
// UX, not the source of truth. Terminal statuses (applied, rejected, superseded,
// withdrawn, expired) have no outgoing transitions.
var legalTransitions = map[string][]string{
	"draft":           {"open", "withdrawn", "expired"},
	"open":            {"in_review", "request_changes", "blocked", "withdrawn", "superseded", "expired"},
	"in_review":       {"approved", "rejected", "request_changes", "blocked", "withdrawn", "superseded", "expired"},
	"request_changes": {"draft", "open", "withdrawn", "superseded", "expired"},
	"blocked":         {"open", "in_review", "rejected", "withdrawn", "superseded", "expired"},
	"approved":        {"applied", "superseded", "expired"},
}

func canTransition(from, to string) bool {
	for _, t := range legalTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// proposalAction maps a key to a governed proposal action.
type proposalAction struct {
	key    string
	label  string
	status string // target transition status; "" for apply (special)
	apply  bool
}

// proposalActions is the documented action set for the Proposals page.
var proposalActions = []proposalAction{
	{key: "o", label: "open", status: "open"},
	{key: "v", label: "submit review", status: "in_review"},
	{key: "a", label: "approve", status: "approved"},
	{key: "c", label: "request changes", status: "request_changes"},
	{key: "x", label: "reject", status: "rejected"},
	{key: "b", label: "block", status: "blocked"},
	{key: "A", label: "apply", apply: true},
	{key: "w", label: "withdraw", status: "withdrawn"},
}

// availableFor returns whether an action is legal for a proposal in the given
// status. Apply is legal only from approved; transitions follow the table.
func (a proposalAction) availableFor(status string) bool {
	if a.apply {
		return status == "approved"
	}
	return canTransition(status, a.status)
}
