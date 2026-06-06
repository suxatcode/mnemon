// Package corebridge is the seam where the host-lifecycle layer feeds the core engine
// (Ring 3 -> Ring 1, via the channel). It adapts the host-lifecycle event model
// (schema.Event, with its rich host fields) to the kernel's ONE canonical event model
// (contract.ObservationEnvelope / contract.Event).
//
// The unification rule (P2.1): contract.Event is the canonical event. schema.Event's
// host-lifecycle-only fields (Loop/Host/Source/Severity/ProposalRef/AuditRef/StatusRef/
// Hashes/Scope/Privacy/...) ride as a TYPED PAYLOAD EXTENSION under a reserved key, not
// as a rival top-level struct — so a host event becomes an envelope/payload over the
// canonical event and reconstructs losslessly.
package corebridge

import (
	"encoding/json"
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// HostExtensionKey is the reserved payload key under which schema.Event's
// host-lifecycle-only fields are carried through the canonical contract.Event so a
// round-trip reconstructs the original schema.Event exactly (modulo the core-assigned
// IngestSeq, which schema.Event does not have). Domain payloads must not use this key.
const HostExtensionKey = "_host_lifecycle"

// hostExtension is the typed carrier for the schema.Event fields that have no home in
// contract.Event's top-level shape. It is JSON-encoded into the canonical payload.
type hostExtension struct {
	SchemaVersion int                `json:"schema_version"`
	Loop          *string            `json:"loop"`
	Host          *string            `json:"host"`
	Source        string             `json:"source"`
	ProjectRoot   string             `json:"project_root,omitempty"`
	Store         string             `json:"store,omitempty"`
	Scope         map[string]any     `json:"scope,omitempty"`
	Severity      string             `json:"severity,omitempty"`
	Privacy       map[string]any     `json:"privacy,omitempty"`
	ArtifactRefs  []schema.RawObject `json:"artifact_refs,omitempty"`
	StatusRef     map[string]any     `json:"status_ref,omitempty"`
	ProposalRef   map[string]any     `json:"proposal_ref,omitempty"`
	AuditRef      map[string]any     `json:"audit_ref,omitempty"`
	Hashes        map[string]any     `json:"hashes,omitempty"`
}

// ToEnvelope lowers a host-lifecycle schema.Event into a contract.ObservationEnvelope
// addressed to the canonical log. The host fields are packed under HostExtensionKey; the
// domain payload keys ride alongside, untouched. Source becomes the observation principal
// and the lifecycle event ID becomes the idempotency ExternalID.
func ToEnvelope(ev schema.Event) (contract.ObservationEnvelope, error) {
	extMap, err := structToMap(hostExtension{
		SchemaVersion: ev.SchemaVersion,
		Loop:          ev.Loop,
		Host:          ev.Host,
		Source:        ev.Source,
		ProjectRoot:   ev.ProjectRoot,
		Store:         ev.Store,
		Scope:         ev.Scope,
		Severity:      ev.Severity,
		Privacy:       ev.Privacy,
		ArtifactRefs:  ev.ArtifactRefs,
		StatusRef:     ev.StatusRef,
		ProposalRef:   ev.ProposalRef,
		AuditRef:      ev.AuditRef,
		Hashes:        ev.Hashes,
	})
	if err != nil {
		return contract.ObservationEnvelope{}, err
	}
	payload := make(map[string]any, len(ev.Payload)+1)
	for k, v := range ev.Payload {
		if k == HostExtensionKey {
			return contract.ObservationEnvelope{}, fmt.Errorf("corebridge: domain payload must not use reserved key %q", HostExtensionKey)
		}
		payload[k] = v
	}
	payload[HostExtensionKey] = extMap

	causedBy := ""
	if ev.CausedBy != nil {
		causedBy = *ev.CausedBy
	}
	return contract.ObservationEnvelope{
		Source:     contract.ActorID(ev.Source),
		ExternalID: ev.ID,
		Event: contract.Event{
			SchemaVersion: 1, // the canonical contract.Event schema version (kernel rejects others)
			ID:            ev.ID,
			TS:            ev.TS,
			Type:          ev.Type,
			Actor:         contract.ActorID(ev.Actor),
			CorrelationID: ev.CorrelationID,
			CausedBy:      causedBy,
			Payload:       payload,
		},
	}, nil
}

// FromEvent reconstructs a host-lifecycle schema.Event from a canonical contract.Event:
// the host fields are read back out of HostExtensionKey and the remaining payload keys are
// the domain payload. The core-assigned IngestSeq is dropped (schema.Event has no slot).
func FromEvent(ev contract.Event) (schema.Event, error) {
	out := schema.Event{
		SchemaVersion: ev.SchemaVersion,
		ID:            ev.ID,
		TS:            ev.TS,
		Type:          ev.Type,
		Actor:         string(ev.Actor),
		CorrelationID: ev.CorrelationID,
	}
	if ev.CausedBy != "" {
		c := ev.CausedBy
		out.CausedBy = &c
	}
	payload := map[string]any{}
	for k, v := range ev.Payload {
		if k == HostExtensionKey {
			continue
		}
		payload[k] = v
	}
	out.Payload = payload
	if raw, ok := ev.Payload[HostExtensionKey]; ok {
		var ext hostExtension
		if err := mapToStruct(raw, &ext); err != nil {
			return schema.Event{}, fmt.Errorf("corebridge: decode host extension: %w", err)
		}
		out.SchemaVersion = ext.SchemaVersion
		out.Loop = ext.Loop
		out.Host = ext.Host
		out.Source = ext.Source
		out.ProjectRoot = ext.ProjectRoot
		out.Store = ext.Store
		out.Scope = ext.Scope
		out.Severity = ext.Severity
		out.Privacy = ext.Privacy
		out.ArtifactRefs = ext.ArtifactRefs
		out.StatusRef = ext.StatusRef
		out.ProposalRef = ext.ProposalRef
		out.AuditRef = ext.AuditRef
		out.Hashes = ext.Hashes
	}
	return out, nil
}

func structToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func mapToStruct(raw any, out any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
