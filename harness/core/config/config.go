package config

import "github.com/mnemon-dev/mnemon/harness/core/contract"

// ResolvedBinding carries the trusted write identity (Actor) and authorized emit
// type for a binding. The server builds it when stamping a rule/job proposal into a
// *.proposed event via runtime.Bridge.Stamp, which reads only Actor/Emits.
//
// The legacy callback dispatch path (config.Resolve + a Callback proposer field on
// this struct, plus RuntimeConfig/BindingConfig/ModeConfig/Resolved) was removed in
// P0.2: it was superseded by the rule pre-gate. Rule admission now flows through
// ResolveRules (rule_config.go); reconcile mode selection through reconcile.ResolveModes.
type ResolvedBinding struct {
	EventType string
	Actor     contract.ActorID
	Emits     string
}
