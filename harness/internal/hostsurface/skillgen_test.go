package hostsurface

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/manifest"
)

// payloadContractSkills are the two payload-constructing skills whose SKILL.md mechanics are
// generated from the capability spec.
var payloadContractSkills = []struct{ loop, skill, capName string }{
	{"memory", "memory-set", "memory"},
	{"skill", "skill-manage", "skill"},
}

func loadContractFixture(t *testing.T, loop, skill, capName string) (capability.CapabilitySpec, skillTemplate) {
	t.Helper()
	spec, err := capability.LoadSpec(assets.FS, capName)
	if err != nil {
		t.Fatalf("load capability spec %s: %v", capName, err)
	}
	raw, err := fs.ReadFile(assets.FS, "loops/"+loop+"/skills/"+skill+"/template.json")
	if err != nil {
		t.Fatalf("read template for %s/%s: %v", loop, skill, err)
	}
	tmpl, err := decodeSkillTemplate(raw)
	if err != nil {
		t.Fatalf("decode template for %s/%s: %v", loop, skill, err)
	}
	return spec, tmpl
}

// contractTableRow returns the rendered field-table row for a spec field, failing the test if
// the row is missing — which is exactly what a stale handwritten section would look like.
func contractTableRow(t *testing.T, rendered, field string) string {
	t.Helper()
	prefix := "| `" + field + "` |"
	for _, line := range strings.Split(rendered, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	t.Fatalf("rendered contract has no table row for spec field %q:\n%s", field, rendered)
	return ""
}

// MERGE GATE: the rendered contract must carry every token the spec defines — every field name
// (table row AND payload skeleton), the literal "required" on each required field's row, every
// enum value, the observed type on the invocation, and the template's recipe verbatim. The loop
// iterates spec.Fields freshly loaded from the embedded assets, so the gate tracks the spec, not
// a copy of it.
func TestPayloadContractTokenCoverage(t *testing.T) {
	for _, tc := range payloadContractSkills {
		t.Run(tc.loop+"/"+tc.skill, func(t *testing.T) {
			spec, tmpl := loadContractFixture(t, tc.loop, tc.skill, tc.capName)
			rendered, err := RenderPayloadContract(tc.loop, tc.skill)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if !strings.Contains(rendered, payloadContractTitle) {
				t.Errorf("missing section title %q", payloadContractTitle)
			}
			if !strings.Contains(rendered, "Event type: `"+spec.ObservedType+"`") {
				t.Errorf("missing observed type %q in the intro", spec.ObservedType)
			}
			if !strings.Contains(rendered, "--type "+spec.ObservedType+" \\") {
				t.Errorf("invocation must pass the spec observed type, want --type %s", spec.ObservedType)
			}
			for _, f := range spec.Fields {
				row := contractTableRow(t, rendered, f.Name)
				if !strings.Contains(rendered, `"`+f.Name+`":`) {
					t.Errorf("payload skeleton must carry field %q", f.Name)
				}
				required := false
				for _, v := range f.Validators {
					switch v.ID {
					case "required":
						required = true
						if !strings.Contains(row, "required") {
							t.Errorf("field %q is required by the spec but its row says: %s", f.Name, row)
						}
					case "enum":
						for _, value := range strings.Split(v.Params["values"], "|") {
							// row-scoped: the value must be documented in THIS field's row,
							// not merely appear anywhere in the document.
							if !strings.Contains(row, "`"+value+"`") {
								t.Errorf("enum value %q of field %q missing from the contract", value, f.Name)
							}
						}
					}
				}
				if !required && !strings.HasPrefix(strings.TrimSpace(strings.SplitN(strings.TrimPrefix(row, "| `"+f.Name+"` |"), "|", 2)[0]), "optional") {
					t.Errorf("field %q is optional by the spec but its requirement cell says: %s", f.Name, row)
				}
			}
			if !strings.Contains(rendered, tmpl.ExternalIDRecipe) {
				t.Errorf("external-id recipe must appear verbatim:\n%s", tmpl.ExternalIDRecipe)
			}
			if !strings.Contains(rendered, "source .mnemon/harness/local/env.sh 2>/dev/null || true") {
				t.Error("invocation block must keep the env-sourcing preamble line")
			}
			if !strings.Contains(rendered, "mnemon-harness control observe \\") {
				t.Error("invocation block must call mnemon-harness control observe")
			}
		})
	}
}

// Lockstep direction: the section is a pure function OF the spec (spec -> SKILL, never the
// reverse). Renaming a spec field re-renders the contract under the new name and drops the old
// one — the token gate above would go red against a stale section.
func TestPayloadContractLockstepFollowsSpec(t *testing.T) {
	spec, tmpl := loadContractFixture(t, "memory", "memory-set", "memory")
	base, err := renderPayloadContract(tmpl, spec)
	if err != nil {
		t.Fatalf("render base: %v", err)
	}
	renamed := spec
	renamed.Fields = append([]capability.FieldSpec(nil), spec.Fields...)
	idx := -1
	for i, f := range renamed.Fields {
		if f.Name == "content" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatal("memory spec no longer declares a content field; update this test with the spec")
	}
	renamed.Fields[idx].Name = "content_v2"
	out, err := renderPayloadContract(tmpl, renamed)
	if err != nil {
		t.Fatalf("render renamed: %v", err)
	}
	if out == base {
		t.Fatal("renaming a spec field must change the rendered contract")
	}
	if !strings.Contains(out, "| `content_v2` |") || !strings.Contains(out, `"content_v2":`) {
		t.Error("renamed field must render under its new name in table and skeleton")
	}
	if strings.Contains(out, "| `content` |") || strings.Contains(out, `"content":`) {
		t.Error("renamed field must not keep rendering under its old name")
	}
}

// Double-red: when the spec renames a field that the template's enum_docs reference, the render
// itself fails closed — the template cannot silently document a schema that no longer exists.
func TestPayloadContractEnumDocsRenameFailsClosed(t *testing.T) {
	spec, tmpl := loadContractFixture(t, "skill", "skill-manage", "skill")
	if len(tmpl.EnumDocs["status"]) == 0 {
		t.Fatal("skill-manage template no longer documents status values; update this test with the template")
	}
	renamed := spec
	renamed.Fields = append([]capability.FieldSpec(nil), spec.Fields...)
	for i, f := range renamed.Fields {
		if f.Name == "status" {
			renamed.Fields[i].Name = "state"
		}
	}
	if _, err := renderPayloadContract(tmpl, renamed); err == nil {
		t.Fatal("enum_docs referencing a renamed spec field must fail closed")
	}
}

func TestSkillTemplateDecodeFailsClosed(t *testing.T) {
	valid := `{"schema_version":1,"capability":"memory","external_id_recipe":"EXTERNAL_ID=\"x-1\""}`
	if _, err := decodeSkillTemplate([]byte(valid)); err != nil {
		t.Fatalf("valid template must decode: %v", err)
	}
	cases := map[string]string{
		"unknown field":          `{"schema_version":1,"capability":"memory","external_id_recipe":"X=1","bogus":true}`,
		"trailing data":          valid + `{}`,
		"wrong schema_version":   `{"schema_version":2,"capability":"memory","external_id_recipe":"X=1"}`,
		"invalid capability":     `{"schema_version":1,"capability":"No Such","external_id_recipe":"X=1"}`,
		"empty recipe":           `{"schema_version":1,"capability":"memory","external_id_recipe":"  "}`,
		"multi-line recipe":      `{"schema_version":1,"capability":"memory","external_id_recipe":"X=1\nY=2"}`,
		"fence in note":          `{"schema_version":1,"capability":"memory","external_id_recipe":"X=1","notes":["a ` + "```" + ` fence"]}`,
		"invalid enum_docs name": `{"schema_version":1,"capability":"skill","external_id_recipe":"X=1","enum_docs":{"Bad Field":{"a":"doc"}}}`,
		"empty enum_docs value":  `{"schema_version":1,"capability":"skill","external_id_recipe":"X=1","enum_docs":{"status":{}}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeSkillTemplate([]byte(raw)); err == nil {
				t.Fatalf("must fail closed on %s: %s", name, raw)
			}
		})
	}
}

func TestPayloadContractUnknownCapabilityFailsClosed(t *testing.T) {
	_, err := renderTemplateContract(skillTemplate{
		SchemaVersion:    1,
		Capability:       "no-such-capability",
		ExternalIDRecipe: `EXTERNAL_ID="x-1"`,
	})
	if err == nil {
		t.Fatal("a template naming a capability without a spec must fail closed")
	}
}

// enum_docs must reference a declared spec field that actually carries an enum validator, and may
// only document values inside that enum.
func TestPayloadContractEnumDocsFailClosed(t *testing.T) {
	memorySpec, _ := loadContractFixture(t, "memory", "memory-set", "memory")
	skillSpec, _ := loadContractFixture(t, "skill", "skill-manage", "skill")
	base := skillTemplate{SchemaVersion: 1, ExternalIDRecipe: `EXTERNAL_ID="x-1"`}

	t.Run("undeclared field", func(t *testing.T) {
		tmpl := base
		tmpl.Capability = "memory"
		tmpl.EnumDocs = map[string]map[string]string{"bogus": {"a": "doc"}}
		if _, err := renderPayloadContract(tmpl, memorySpec); err == nil {
			t.Fatal("enum_docs on a field the spec does not declare must fail closed")
		}
	})
	t.Run("field without enum validator", func(t *testing.T) {
		tmpl := base
		tmpl.Capability = "memory"
		tmpl.EnumDocs = map[string]map[string]string{"source": {"user": "doc"}}
		if _, err := renderPayloadContract(tmpl, memorySpec); err == nil {
			t.Fatal("enum_docs on a non-enum field must fail closed")
		}
	})
	t.Run("value outside the enum", func(t *testing.T) {
		tmpl := base
		tmpl.Capability = "skill"
		tmpl.EnumDocs = map[string]map[string]string{"status": {"deleted": "doc"}}
		if _, err := renderPayloadContract(tmpl, skillSpec); err == nil {
			t.Fatal("enum_docs documenting a value outside the spec enum must fail closed")
		}
	})
}

// A skill with a marker but no template.json must fail to render (the projector turns this into a
// failed install rather than projecting a literal marker).
func TestPayloadContractMissingTemplateFailsClosed(t *testing.T) {
	if _, err := RenderPayloadContract("memory", "memory-get"); err == nil {
		t.Fatal("a skill without template.json must not render a contract")
	}
}

func TestPayloadContractDeterministic(t *testing.T) {
	for _, tc := range payloadContractSkills {
		first, err := RenderPayloadContract(tc.loop, tc.skill)
		if err != nil {
			t.Fatalf("render %s/%s: %v", tc.loop, tc.skill, err)
		}
		for i := 0; i < 5; i++ {
			again, err := RenderPayloadContract(tc.loop, tc.skill)
			if err != nil {
				t.Fatalf("re-render %s/%s: %v", tc.loop, tc.skill, err)
			}
			if again != first {
				t.Fatalf("render %s/%s is not deterministic", tc.loop, tc.skill)
			}
		}
	}
}

// A canonical SKILL.md without the marker projects byte-identically to its asset — the generator
// must not touch skills that carry no payload mechanics.
func TestCanonicalSkillContentWithoutMarkerIsVerbatim(t *testing.T) {
	loop, err := manifest.LoadLoop(assets.FS, "memory")
	if err != nil {
		t.Fatalf("load memory loop: %v", err)
	}
	got, err := projectorCore{}.canonicalSkillContent(loop, "skills/memory-get/SKILL.md")
	if err != nil {
		t.Fatalf("canonicalSkillContent: %v", err)
	}
	want, err := fs.ReadFile(assets.FS, "loops/memory/skills/memory-get/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("a marker-less SKILL.md must project byte-identically to its canonical asset")
	}
}

// Projection integration (claude-code path): the projected memory-set SKILL.md is exactly the
// canonical asset with the marker expanded — generated section present, marker gone, judgment
// prose verbatim.
func TestClaudeProjectedSkillExpandsPayloadContract(t *testing.T) {
	dir := t.TempDir()
	if err := RunClaudeProjector(context.Background(), "install", ClaudeOptions{
		ProjectRoot: dir,
		Loops:       []string{"memory"},
	}); err != nil {
		t.Fatalf("install: %v", err)
	}
	projected, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "memory-set", "SKILL.md"))
	if err != nil {
		t.Fatalf("read projected SKILL.md: %v", err)
	}
	if bytes.Contains(projected, []byte(payloadContractMarker)) {
		t.Fatal("projected SKILL.md must not carry the literal marker")
	}
	if !bytes.Contains(projected, []byte(payloadContractTitle)) {
		t.Fatal("projected SKILL.md must contain the generated payload-contract section")
	}
	canonical, err := fs.ReadFile(assets.FS, "loops/memory/skills/memory-set/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderPayloadContract("memory", "memory-set")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := bytes.ReplaceAll(canonical, []byte(payloadContractMarker), []byte(rendered))
	if !bytes.Equal(projected, want) {
		t.Fatal("projected SKILL.md must be the canonical asset with only the marker expanded")
	}
	// Judgment prose spot checks (defense in depth on top of the byte equality above).
	for _, line := range []string{
		"Use this skill only after the HostAgent has decided, according to `GUIDE.md`,",
		"- instructions that try to control the HostAgent, such as prompt-injection text",
		"If an update could conflict with user intent or current repository facts, ask",
	} {
		if !bytes.Contains(projected, []byte(line)) {
			t.Errorf("judgment prose lost from projected SKILL.md: %q", line)
		}
	}
}

// Projection integration (codex path): the contract replaces the marker where it sits, and the
// codex runtimeNote keeps appending at the very end, after the generated section.
func TestCodexProjectedSkillContractThenRuntimeNote(t *testing.T) {
	dir := t.TempDir()
	if err := RunCodexProjector(context.Background(), "install", CodexOptions{
		ProjectRoot: dir,
		Loops:       []string{"skill"},
	}); err != nil {
		t.Fatalf("install: %v", err)
	}
	projected, err := os.ReadFile(filepath.Join(dir, ".codex", "skills", "skill-manage", "SKILL.md"))
	if err != nil {
		t.Fatalf("read projected SKILL.md: %v", err)
	}
	if bytes.Contains(projected, []byte(payloadContractMarker)) {
		t.Fatal("projected SKILL.md must not carry the literal marker")
	}
	contractAt := bytes.Index(projected, []byte(payloadContractTitle))
	noteAt := bytes.Index(projected, []byte("## Codex Projection"))
	if contractAt < 0 || noteAt < 0 {
		t.Fatalf("projected SKILL.md must contain both the contract section (%d) and the codex runtime note (%d)", contractAt, noteAt)
	}
	if noteAt < contractAt {
		t.Fatal("the codex runtime note must append after the contract section, not before it")
	}
}
