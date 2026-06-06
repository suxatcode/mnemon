package hostsurface

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/status"
)

// installCodexMemory installs (or re-projects) the memory loop onto a project's
// codex surface using the shared fixture declaration.
func installCodexMemory(t *testing.T, root, projectRoot string) {
	t.Helper()
	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		DeclarationRoot: root, ProjectRoot: projectRoot, Loops: []string{"memory"}, Stdout: &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("install: %v", err)
	}
}

func envelopePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".codex", "mnemon-memory", projectionEnvelopeFile)
}

func readEnvelope(t *testing.T, projectRoot string) ProjectionEnvelope {
	t.Helper()
	data, err := os.ReadFile(envelopePath(projectRoot))
	if err != nil {
		t.Fatalf("read %s: %v", projectionEnvelopeFile, err)
	}
	var env ProjectionEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("parse envelope: %v", err)
	}
	return env
}

func envelopeFragmentPresent(env ProjectionEnvelope, kind string) bool {
	for _, f := range env.Fragments {
		if f.Kind == kind {
			return f.Present
		}
	}
	return false
}

// codexReadbackEchoing reads the real event log, appends ONE synthetic host-agent
// writeback echoing echoDigest (as a real Codex turn would, reading it from
// PROJECTION.json), and returns codex's verifier readback. The projection.applied
// baseline is real (from install); only the host echo is synthesized — a real
// host turn is the manual dogfood, not this deterministic gate.
func codexReadbackEchoing(t *testing.T, projectRoot, echoDigest string) status.HostReadback {
	t.Helper()
	store, err := eventlog.New(projectRoot)
	if err != nil {
		t.Fatalf("eventlog.New: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	host, loop := "codex", "memory"
	events = append(events, schema.Event{
		SchemaVersion: schema.Version,
		ID:            "evt_test_host_echo",
		TS:            "2026-05-31T12:00:00Z",
		Type:          "memory.hot_write_observed",
		Loop:          &loop,
		Host:          &host,
		Actor:         "host-agent",
		Source:        "host",
		Payload:       map[string]any{"observed_context_digest": echoDigest, "reason": "acted on pulled context"},
	})
	for _, rb := range status.DeriveReadback(events) {
		if rb.Host == "codex" {
			return rb
		}
	}
	t.Fatalf("no codex readback derived")
	return status.HostReadback{}
}

// TestProjectionEnvelopeBaselineWithoutContent is dogfood finding #1: a fresh
// install with NO profile content still writes PROJECTION.json AND emits a
// projection.applied baseline — the projection ACT happened, so the writeback
// verifier has an anchor from the very first install (not coupled to content).
func TestProjectionEnvelopeBaselineWithoutContent(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writePlanFixture(t, root)
	// deliberately seed no profile entry — empty context

	installCodexMemory(t, root, projectRoot)

	env := readEnvelope(t, projectRoot)
	if env.ContextDigest == "" {
		t.Fatal("empty-context envelope must still carry a context_digest")
	}
	if got := projectionAppliedOfKind(t, projectRoot, FragmentProjection); len(got) != 1 {
		t.Fatalf("empty-profile install must emit exactly 1 projection.applied baseline, got %d", len(got))
	}
	if envelopeFragmentPresent(env, FragmentProfile) || envelopeFragmentPresent(env, FragmentCoordination) {
		t.Error("an empty install must report its fragments absent (present=false), not omit the baseline")
	}
}

// TestProjectionEnvelopeMatchesEvent is dogfood finding #2: the digest the host
// must echo is ON ITS SURFACE. PROJECTION.json carries the same projection_ref +
// context_digest as the projection.applied event, so a host echoes a value it can
// actually read — never spelunking .mnemon for its own digest.
func TestProjectionEnvelopeMatchesEvent(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writePlanFixture(t, root)
	seedProfileEntry(t, projectRoot, "pref-one", time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), "codex", "memory")

	installCodexMemory(t, root, projectRoot)

	env := readEnvelope(t, projectRoot)
	if !strings.HasPrefix(env.ContextDigest, "sha256:") {
		t.Errorf("context_digest should be a sha256 hash, got %q", env.ContextDigest)
	}
	if !strings.HasSuffix(env.ProjectionRef, projectionEnvelopeFile) {
		t.Errorf("projection_ref should point at the envelope surface, got %q", env.ProjectionRef)
	}
	if !envelopeFragmentPresent(env, FragmentProfile) {
		t.Error("PROFILE fragment should be present after seeding an entry")
	}

	got := projectionAppliedOfKind(t, projectRoot, FragmentProjection)
	if len(got) != 1 {
		t.Fatalf("want 1 projection.applied baseline, got %d", len(got))
	}
	if d := projectionField(got[0], "context_digest"); d != env.ContextDigest {
		t.Errorf("event digest %q must equal the on-surface envelope digest %q", d, env.ContextDigest)
	}
	if r := projectionField(got[0], "projection_ref"); r != env.ProjectionRef {
		t.Errorf("event ref %q must equal the envelope ref %q", r, env.ProjectionRef)
	}
}

// TestProjectionEnvelopeIdempotent: re-projecting unchanged content emits NO new
// projection.applied and does not rewrite PROJECTION.json (byte-identical).
func TestProjectionEnvelopeIdempotent(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writePlanFixture(t, root)
	seedProfileEntry(t, projectRoot, "pref-one", time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), "codex", "memory")

	installCodexMemory(t, root, projectRoot)
	before, err := os.ReadFile(envelopePath(projectRoot))
	if err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	if n := len(projectionAppliedOfKind(t, projectRoot, FragmentProjection)); n != 1 {
		t.Fatalf("want 1 baseline after first install, got %d", n)
	}

	installCodexMemory(t, root, projectRoot) // unchanged content

	after, err := os.ReadFile(envelopePath(projectRoot))
	if err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("re-projecting unchanged content must not rewrite PROJECTION.json")
	}
	if n := len(projectionAppliedOfKind(t, projectRoot, FragmentProjection)); n != 1 {
		t.Errorf("re-projecting unchanged content must emit no new projection.applied, got %d", n)
	}
}

// TestProjectionContextDigestDeterministic: the same payload yields the same
// digest across runs (no act timestamp leaks into the digest), the empty-context
// digest is defined + stable, and non-empty content differs from empty.
func TestProjectionContextDigestDeterministic(t *testing.T) {
	projectRoot := t.TempDir()

	empty1, _, _, err := projectionContextDigest(projectRoot, "codex", "memory")
	if err != nil {
		t.Fatalf("digest (empty): %v", err)
	}
	empty2, _, _, err := projectionContextDigest(projectRoot, "codex", "memory")
	if err != nil {
		t.Fatalf("digest (empty): %v", err)
	}
	if empty1 == "" || empty1 != empty2 {
		t.Fatalf("empty-context digest must be defined and stable, got %q / %q", empty1, empty2)
	}

	seedProfileEntry(t, projectRoot, "pref-one", time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), "codex", "memory")
	d1, hasProf, _, err := projectionContextDigest(projectRoot, "codex", "memory")
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	d2, _, _, err := projectionContextDigest(projectRoot, "codex", "memory")
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if !hasProf {
		t.Fatal("seeded profile entry should be present in the digest input")
	}
	if d1 != d2 {
		t.Errorf("same payload must yield the same digest, got %q / %q", d1, d2)
	}
	if d1 == empty1 {
		t.Error("non-empty content must differ from the empty-context digest")
	}
}

// TestProjectionEnvelopeVerifierObservedThenStale wires the whole loop: install
// (envelope digest D1) → host echoes D1 read from PROJECTION.json → verifier
// scores observed. A profile change + reproject makes a new live digest D2; the
// host's old D1 echo now reads observed-but-stale. This is finding #3 + #4's
// mechanism, made deterministic.
func TestProjectionEnvelopeVerifierObservedThenStale(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writePlanFixture(t, root)
	seedProfileEntry(t, projectRoot, "pref-one", time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), "codex", "memory")

	installCodexMemory(t, root, projectRoot)
	d1 := readEnvelope(t, projectRoot).ContextDigest

	if rb := codexReadbackEchoing(t, projectRoot, d1); rb.State != status.ReadbackObserved || rb.Stale {
		t.Fatalf("host echoing the live digest should be observed (not stale), got state=%s stale=%v", rb.State, rb.Stale)
	}

	// Reproject with changed content → a new live digest.
	seedProfileEntry(t, projectRoot, "pref-two", time.Date(2026, 5, 31, 0, 0, 1, 0, time.UTC), "codex", "memory")
	installCodexMemory(t, root, projectRoot)
	d2 := readEnvelope(t, projectRoot).ContextDigest
	if d2 == d1 {
		t.Fatal("changed content must change the live digest")
	}
	if n := len(projectionAppliedOfKind(t, projectRoot, FragmentProjection)); n != 2 {
		t.Fatalf("a changed projection must emit a second baseline, got %d", n)
	}

	// The host's last echo is still d1 → observed but stale (acting on old context).
	if rb := codexReadbackEchoing(t, projectRoot, d1); rb.State != status.ReadbackObserved || !rb.Stale {
		t.Fatalf("after reproject, the old echo should be observed+stale, got state=%s stale=%v", rb.State, rb.Stale)
	}
}
