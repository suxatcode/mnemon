// Package supervisor is the pluggable, advisory coordination supervisor.
//
// Mnemon supplies the structured world (the read contract, Context) and the
// proposal contract (the write contract, Suggestion). The supervisor BRAIN —
// what it proposes — is swappable by config, not code: FromConfig selects an
// implementation by kind. A supervisor only PROPOSES; it never mutates the
// topology. The facade turns each Suggestion into a route=coordination proposal
// through the existing proposal → review → apply → audit path.
//
// The rule stand-in here is the deterministic implementation used for local
// validation. A real host-agent supervisor (Codex, Claude, or custom) runs
// externally via the daemon, runner, and host path and calls the same write
// contract. Mnemon never runs the agent brain in-core.
package supervisor

import (
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
)

// Operation names for coordination suggestions — the narrow topology operations
// the apply executor knows how to apply.
const (
	OpMerge        = "coordination.merge"
	OpMarkConflict = "coordination.mark_conflict"
)

// Context is the supervisor read contract: the structured coordination world it
// reasons over. Mnemon assembles it; the brain only reads.
type Context struct {
	Topology      coordination.View `json:"topology"`
	OpenProposals []OpenProposal    `json:"open_proposals,omitempty"`
}

// OpenProposal is a proposal already awaiting review, so the supervisor does not
// duplicate a suggestion already in the queue.
type OpenProposal struct {
	ID        string `json:"id"`
	Route     string `json:"route"`
	Status    string `json:"status"`
	TargetURI string `json:"target_uri,omitempty"`
}

// Suggestion is the supervisor write contract: one advisory coordination change.
// It is data only — the facade converts it into a governed route=coordination
// proposal. The supervisor never applies it.
type Suggestion struct {
	ProposalID   string         `json:"proposal_id"`
	Title        string         `json:"title"`
	Summary      string         `json:"summary"`
	Operation    string         `json:"operation"`
	TargetURI    string         `json:"target_uri"`
	EvidenceRefs []string       `json:"evidence_refs,omitempty"`
	Payload      map[string]any `json:"payload,omitempty"`
}

// Supervisor is the swappable brain: read the Context, propose changes.
type Supervisor interface {
	Name() string
	Propose(Context) []Suggestion
}

// Kind values select an implementation.
const (
	KindRule      = "rule-standin"
	KindHostAgent = "host-agent"
)

// Config selects the supervisor implementation. Swapping the supervisor is a
// config change, not a code change at the call site.
type Config struct {
	Kind string `json:"kind"`
}

// FromConfig returns the supervisor implementation for the configured kind.
func FromConfig(cfg Config) (Supervisor, error) {
	switch strings.TrimSpace(cfg.Kind) {
	case "", KindRule:
		return RuleStandin{}, nil
	case KindHostAgent:
		return nil, fmt.Errorf("supervisor kind %q runs externally via daemon→runner→host (real-host follow-up); not available in-core", cfg.Kind)
	default:
		return nil, fmt.Errorf("unknown supervisor kind %q", cfg.Kind)
	}
}

// RuleStandin is the deterministic test stand-in: from the topology alone it
// proposes merging duplicate work (tasks sharing evidence). Advisory only — it
// returns Suggestions and never mutates the topology.
type RuleStandin struct{}

func (RuleStandin) Name() string { return KindRule }

func (RuleStandin) Propose(ctx Context) []Suggestion {
	taken := map[string]bool{}
	for _, p := range ctx.OpenProposals {
		if p.TargetURI != "" {
			taken[p.TargetURI] = true
		}
	}
	var out []Suggestion
	for _, mc := range ctx.Topology.MergeCandidates {
		if len(mc.Tasks) < 2 {
			continue
		}
		target := "coordination:merge/" + strings.Join(mc.Tasks, "+")
		if taken[target] {
			continue // already proposed and awaiting review; do not duplicate
		}
		tasks := make([]any, len(mc.Tasks))
		for i, t := range mc.Tasks {
			tasks[i] = t
		}
		out = append(out, Suggestion{
			ProposalID:   "sup-merge-" + strings.Join(mc.Tasks, "-"),
			Title:        "Merge duplicate work: " + strings.Join(mc.Tasks, ", "),
			Summary:      "Tasks " + strings.Join(mc.Tasks, ", ") + " share evidence " + mc.EvidenceRef + " — likely duplicate work. Propose a governed merge for human review.",
			Operation:    OpMerge,
			TargetURI:    target,
			EvidenceRefs: []string{mc.EvidenceRef},
			Payload:      map[string]any{"operation": "merge", "tasks": tasks, "into": mc.Tasks[0]},
		})
	}
	return out
}
