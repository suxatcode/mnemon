package hostsurface

import (
	"encoding/json"
	"os"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/declaration"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/profile"
)

// The Projection Envelope makes the push side of the access loop verifiable from
// the host surface alone. PROFILE.json and COORDINATION.json stay the payload;
// PROJECTION.json is the metadata envelope that carries the provenance the host
// must echo (`projection_ref` + `context_digest`). The projection act always
// emits provenance, even when no scoped payload exists, and the digest is written
// where the host can read it.
//
// It is a Mnemon-side data contract, not a frozen host adapter interface.

// projectionEnvelopeFile is the metadata envelope written on every host runtime
// surface, beside the GUIDE and the payload fragments.
const projectionEnvelopeFile = "PROJECTION.json"

const (
	projectionEnvelopeSchema = "mnemon.projection_envelope.v1"
	projectionEnvelopeKind   = "ProjectionEnvelope"
)

// FragmentProjection marks the projection ACT (the envelope) on the
// projection.applied event, distinct from the per-fragment payload kinds. It is
// the single provenance baseline per host+loop projection.
const FragmentProjection = "PROJECTION"

// ProjectionEnvelope is the on-surface metadata document (PROJECTION.json). The
// host reads context_digest from here and echoes it on writeback so the verifier
// can score "observed" without the host ever reading canonical .mnemon state.
type ProjectionEnvelope struct {
	SchemaVersion string                  `json:"schema_version"`
	Kind          string                  `json:"kind"`
	Host          string                  `json:"host"`
	Loop          string                  `json:"loop"`
	ProjectionRef string                  `json:"projection_ref"`
	ContextDigest string                  `json:"context_digest"`
	GeneratedAt   string                  `json:"generated_at"`
	Fragments     []ProjectionFragmentRef `json:"fragments"`
}

// ProjectionFragmentRef records each payload fragment the envelope covers and
// whether it is currently present on the surface (absent when nothing is scoped).
type ProjectionFragmentRef struct {
	Kind    string `json:"kind"`
	Ref     string `json:"ref"`
	Present bool   `json:"present"`
}

// projectedContext is the canonical digest INPUT: the dynamic content a host can
// read off its surface (today profile + coordination; future fragments slot in
// here). Field order is fixed and it deliberately holds NO timestamp of the
// projection act and not the envelope's own digest — so the digest is
// deterministic across runs (same content → same digest) and idempotent, with a
// defined empty-context digest (`{}` when nothing is scoped).
type projectedContext struct {
	Profile      *profile.Profile   `json:"profile,omitempty"`
	Coordination *coordination.View `json:"coordination,omitempty"`
}

// projectionContextDigest computes the deterministic context digest for (host,
// loop) over the scoped profile + coordination fragments, and reports which
// fragments are present. It reads canonical state (the same source the payload
// fragments are written from), so the digest matches what the host reads.
func projectionContextDigest(projectRoot, host, loop string) (digest string, hasProfile, hasCoordination bool, err error) {
	var content projectedContext
	prof, ok, perr := scopedProfileFragment(projectRoot, host, loop)
	if perr != nil {
		return "", false, false, perr
	}
	if ok {
		content.Profile = &prof
		hasProfile = true
	}
	coord, ok, cerr := scopedCoordinationFragment(projectRoot, host)
	if cerr != nil {
		return "", false, false, cerr
	}
	if ok {
		content.Coordination = &coord
		hasCoordination = true
	}
	digest, err = fragmentDigest(content)
	return digest, hasProfile, hasCoordination, err
}

// applyProjectionEnvelope writes PROJECTION.json onto the host runtime surface and
// emits ONE projection.applied for the projection ACT — even when profile and
// coordination are both empty/absent (the act still happened; the verifier needs
// a baseline from the first install). It is idempotent at the surface: if the
// envelope already there carries the same context_digest, nothing is rewritten and
// no event is emitted, so re-projecting unchanged content appends nothing.
func (c projectorCore) applyProjectionEnvelope(loop declaration.LoopManifest, binding declaration.BindingManifest) error {
	digest, hasProfile, hasCoordination, err := projectionContextDigest(c.projectRoot, c.host, loop.Name)
	if err != nil {
		return err
	}
	ref := c.displayJoin(binding.RuntimeSurface, projectionEnvelopeFile)

	if existing, ok := c.readEnvelopeDigest(ref); ok && existing == digest {
		return nil // unchanged content — no rewrite, no event
	}

	env := ProjectionEnvelope{
		SchemaVersion: projectionEnvelopeSchema,
		Kind:          projectionEnvelopeKind,
		Host:          c.host,
		Loop:          loop.Name,
		ProjectionRef: ref,
		ContextDigest: digest,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Fragments: []ProjectionFragmentRef{
			{Kind: FragmentProfile, Ref: c.displayJoin(binding.RuntimeSurface, profileFragmentFile), Present: hasProfile},
			{Kind: FragmentCoordination, Ref: c.displayJoin(binding.RuntimeSurface, coordinationFragmentFile), Present: hasCoordination},
		},
	}
	if err := c.writeJSON(ref, env, 0o644); err != nil {
		return err
	}
	return recordProjectionApplied(c.projectRoot, c.host, loop.Name, FragmentProjection, ref, digest)
}

// readEnvelopeDigest returns the context_digest of the envelope currently on the
// surface, if any. Missing/unparsable/empty → not present, which forces a write.
func (c projectorCore) readEnvelopeDigest(ref string) (string, bool) {
	data, err := os.ReadFile(c.resolve(ref))
	if err != nil {
		return "", false
	}
	var env ProjectionEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return "", false
	}
	if env.ContextDigest == "" {
		return "", false
	}
	return env.ContextDigest, true
}
