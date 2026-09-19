package hostsurface

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/suxatcode/mnemon/harness/internal/assets"
	"github.com/suxatcode/mnemon/harness/internal/capability"
	"github.com/suxatcode/mnemon/harness/internal/manifest"
)

// skillgen renders the "Payload contract" section of a payload-constructing SKILL.md from the
// capability spec (loop-package-v1 "SKILL generation rule"). The canonical SKILL.md keeps only
// frontmatter + judgment prose and marks the payload-mechanics position with payloadContractMarker;
// at projection time the marker is replaced by a section derived from:
//
//   - the capability spec (fields, requiredness, defaults, enum values, safety scans, observed
//     type) — the single mechanical source: a spec change re-renders every projected SKILL.md;
//   - skills/<id>/template.json — the teaching-only data the frozen spec does not carry
//     (external-id recipe, per-enum-value docs, caveat notes).
//
// Like the hook generator, the output is a pure function of (template, spec): no clock, no
// environment, no map-iteration order — every map walk goes through sorted keys or the spec's
// declaration order.

// payloadContractMarker is the line a canonical SKILL.md places where the generated payload
// contract belongs. Skills without the marker project byte-identically to their canonical asset.
const payloadContractMarker = "<!-- mnemon:payload-contract -->"

// payloadContractTitle heads the generated section; tests and human diffs key on it.
const payloadContractTitle = "## Payload contract (generated from capability spec)"

// skillTemplateSchemaVersion pins skills/<id>/template.json. Unknown versions fail closed: a
// future format must ship a decoder that understands it, never a silent partial read.
const skillTemplateSchemaVersion = 1

// skillTemplate is the decoded skills/<id>/template.json. It deliberately carries ONLY what the
// capability spec cannot: the spec is a stage-2 frozen surface, so teaching data that would
// otherwise have to live there (recipes, value docs) lives in the loop package instead.
type skillTemplate struct {
	SchemaVersion int    `json:"schema_version"`
	Capability    string `json:"capability"`
	// ExternalIDRecipe is the one-line shell recipe for a stable idempotency key, spliced
	// verbatim into a bash fence.
	ExternalIDRecipe string `json:"external_id_recipe"`
	// EnumDocs documents enum values: field -> value -> one-line doc. Every referenced field must
	// exist in the spec AND carry an enum validator, and every documented value must be one of the
	// enum's values — checked at render time so a spec rename breaks the template loudly.
	EnumDocs map[string]map[string]string `json:"enum_docs,omitempty"`
	// Notes are verbatim one-line caveats worth keeping from the retired handwritten prose.
	Notes []string `json:"notes,omitempty"`
}

// decodeSkillTemplate is the ONE way template.json is read: DisallowUnknownFields + a trailing
// io.EOF check (house rule, same as capability.decodeSpec), then semantic validation. Anything
// it accepts, renderPayloadContract can splice into markdown without breaking the document.
func decodeSkillTemplate(raw []byte) (skillTemplate, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var tmpl skillTemplate
	if err := dec.Decode(&tmpl); err != nil {
		return skillTemplate{}, err
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return skillTemplate{}, fmt.Errorf("trailing data after skill template (want a single JSON object)")
	}
	if err := validateSkillTemplate(tmpl); err != nil {
		return skillTemplate{}, err
	}
	return tmpl, nil
}

func validateSkillTemplate(tmpl skillTemplate) error {
	if tmpl.SchemaVersion != skillTemplateSchemaVersion {
		return fmt.Errorf("unsupported skill template schema_version %d (want %d)", tmpl.SchemaVersion, skillTemplateSchemaVersion)
	}
	if !markerNamePattern.MatchString(tmpl.Capability) {
		return fmt.Errorf("invalid template capability %q", tmpl.Capability)
	}
	if err := validateContractLine(tmpl.ExternalIDRecipe, "external_id_recipe"); err != nil {
		return err
	}
	for _, field := range sortedKeys(tmpl.EnumDocs) {
		if !fieldNamePattern.MatchString(field) {
			return fmt.Errorf("enum_docs: invalid field name %q", field)
		}
		if len(tmpl.EnumDocs[field]) == 0 {
			return fmt.Errorf("enum_docs.%s: no documented values", field)
		}
		for _, value := range sortedKeys(tmpl.EnumDocs[field]) {
			if err := validateContractLine(tmpl.EnumDocs[field][value], fmt.Sprintf("enum_docs.%s.%s", field, value)); err != nil {
				return err
			}
		}
	}
	for i, note := range tmpl.Notes {
		if err := validateContractLine(note, fmt.Sprintf("notes[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

// validateContractLine is the static safety scan for the template's free-text slots: they are
// spliced into markdown (the recipe into a bash fence), so a newline or a fence terminator could
// restructure the projected document. One line, no fences — fail closed otherwise.
func validateContractLine(text, where string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%s: empty", where)
	}
	if strings.ContainsAny(text, "\n\r") {
		return fmt.Errorf("%s: must be a single line", where)
	}
	if strings.Contains(text, "```") {
		return fmt.Errorf("%s: must not contain a markdown fence", where)
	}
	return nil
}

// RenderPayloadContract renders the payload-contract section for loops/<loop>/skills/<skill>:
// template.json supplies the teaching data, capability.LoadSpec the mechanical truth. It is the
// generator entry point — pure with respect to everything except assets.FS.
func RenderPayloadContract(fsys fs.FS, loop, skill string) (string, error) {
	if !markerNamePattern.MatchString(loop) {
		return "", fmt.Errorf("invalid loop name %q", loop)
	}
	if !markerNamePattern.MatchString(skill) {
		return "", fmt.Errorf("invalid skill id %q", skill)
	}
	raw, err := fs.ReadFile(fsys, "loops/"+loop+"/skills/"+skill+"/template.json")
	if err != nil {
		return "", fmt.Errorf("read skill template for %s/%s: %w", loop, skill, err)
	}
	tmpl, err := decodeSkillTemplate(raw)
	if err != nil {
		return "", fmt.Errorf("decode skill template for %s/%s: %w", loop, skill, err)
	}
	return renderTemplateContract(tmpl)
}

// renderTemplateContract resolves the template's capability against the embedded specs. A
// capability name with no spec fails closed here (the template must never render against a
// guessed or stale schema).
func renderTemplateContract(tmpl skillTemplate) (string, error) {
	spec, err := capability.LoadSpec(assets.FS, tmpl.Capability)
	if err != nil {
		return "", fmt.Errorf("template capability %q has no loadable spec (fail-closed): %w", tmpl.Capability, err)
	}
	return renderPayloadContract(tmpl, spec)
}

// renderPayloadContract is the internal seam: section markdown from an explicit (template, spec)
// pair. Tests drive it with modified spec copies to prove the spec→SKILL lockstep direction.
func renderPayloadContract(tmpl skillTemplate, spec capability.CapabilitySpec) (string, error) {
	if spec.SchemaVersion != 1 {
		return "", fmt.Errorf("capability spec %q: schema_version %d unsupported (want 1)", spec.Name, spec.SchemaVersion)
	}
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.ObservedType) == "" {
		return "", fmt.Errorf("capability spec for template %q: missing name or observed_type", tmpl.Capability)
	}
	if len(spec.Fields) == 0 {
		return "", fmt.Errorf("capability spec %q declares no fields; nothing to render", spec.Name)
	}
	declared := map[string]capability.FieldSpec{}
	for _, f := range spec.Fields {
		// Field names are spliced into markdown and a JSON skeleton: keep them inert.
		if !fieldNamePattern.MatchString(f.Name) {
			return "", fmt.Errorf("capability spec %q: field name %q is not renderable (fail-closed)", spec.Name, f.Name)
		}
		declared[f.Name] = f
	}
	// enum_docs must stay in lockstep with the spec: an undeclared field, a field without an enum
	// validator, or a value outside the enum means the template documents a schema that no longer
	// exists — fail closed instead of teaching it.
	for _, field := range sortedKeys(tmpl.EnumDocs) {
		f, ok := declared[field]
		if !ok {
			return "", fmt.Errorf("enum_docs field %q is not declared by capability spec %q (fail-closed)", field, spec.Name)
		}
		values := enumValues(f)
		if values == nil {
			return "", fmt.Errorf("enum_docs field %q has no enum validator in capability spec %q (fail-closed)", field, spec.Name)
		}
		for _, value := range sortedKeys(tmpl.EnumDocs[field]) {
			if !stringInSlice(values, value) {
				return "", fmt.Errorf("enum_docs field %q documents value %q outside the spec enum %s (fail-closed)", field, value, strings.Join(values, "|"))
			}
		}
	}

	blocks := []string{payloadContractTitle}
	blocks = append(blocks, strings.Join([]string{
		"Event type: `" + spec.ObservedType + "` (capability `" + spec.Name + "`).",
		"The payload is ONE JSON object; only the fields below are processed, any other",
		"key is dropped before validation. A failed validator rejects the candidate as",
		"`" + spec.Name + " candidate denied: <reason>`.",
	}, "\n"))

	table := []string{"| Field | Requirement | Constraints |", "| --- | --- | --- |"}
	for _, f := range spec.Fields {
		row, err := payloadFieldRow(spec.Name, f)
		if err != nil {
			return "", err
		}
		table = append(table, row)
	}
	blocks = append(blocks, strings.Join(table, "\n"))

	// Enum value docs, in spec declaration order (fields) and spec enum order (values).
	for _, f := range spec.Fields {
		docs, ok := tmpl.EnumDocs[f.Name]
		if !ok {
			continue
		}
		lines := []string{"`" + f.Name + "` values:", ""}
		for _, value := range enumValues(f) {
			if doc, ok := docs[value]; ok {
				lines = append(lines, "- `"+value+"` — "+doc)
			}
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}

	if len(tmpl.Notes) > 0 {
		lines := []string{"Notes:", ""}
		for _, note := range tmpl.Notes {
			lines = append(lines, "- "+note)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}

	blocks = append(blocks, "Choose a stable idempotency key for this candidate:")
	blocks = append(blocks, "```bash\n"+tmpl.ExternalIDRecipe+"\n```")

	blocks = append(blocks, strings.Join([]string{
		"Submit through the channel, using the Local Mnemon environment installed by",
		"setup when it is available:",
	}, "\n"))
	blocks = append(blocks, strings.Join([]string{
		"```bash",
		`source .mnemon/harness/local/env.sh 2>/dev/null || true`,
		`mnemon-harness control observe \`,
		`  --type ` + spec.ObservedType + ` \`,
		`  --addr "${MNEMON_CONTROL_ADDR:-http://127.0.0.1:8787}" \`,
		`  --principal "${MNEMON_CONTROL_PRINCIPAL}" \`,
		`  ${MNEMON_CONTROL_TOKEN_FILE:+--token-file "${MNEMON_CONTROL_TOKEN_FILE}"} \`,
		`  --external-id "${EXTERNAL_ID}" \`,
		`  --payload '` + payloadSkeleton(spec) + `'`,
		"```",
	}, "\n"))

	return strings.Join(blocks, "\n\n"), nil
}

// payloadFieldRow derives one table row from a field's validators. The switch is CLOSED over the
// compiled validator catalog: a future catalog member must get a teaching rendering here before a
// contract-bearing skill's spec may select it — silently omitting a constraint would teach the
// HostAgent a looser contract than the channel enforces.
func payloadFieldRow(specName string, f capability.FieldSpec) (string, error) {
	requirement := "optional"
	var constraints []string
	for _, v := range f.Validators {
		switch v.ID {
		case "required":
			requirement = "required"
			constraints = append(constraints, "blank denied: `"+v.Params["missing_style"]+" "+f.Name+"`")
		case "default":
			requirement = "optional, default `" + v.Params["value"] + "`"
		case "default-from":
			requirement = "optional, defaults from `" + v.Params["field"] + "`"
		case "enum":
			// Comma-joined: a raw "|" inside a markdown table cell would split the row.
			values := strings.Split(v.Params["values"], "|")
			constraints = append(constraints, "one of `"+strings.Join(values, "`, `")+"`, otherwise denied: `"+v.Params["message"]+"`")
		case "format:skill-id":
			constraints = append(constraints, "lowercase letters, digits, and `-` only, otherwise denied: `invalid "+f.Name+"`")
		case "safety:secret":
			constraints = append(constraints, "secret-like content denied: `secret-like content`")
		case "safety:injection":
			constraints = append(constraints, "prompt-injection-shaped content denied: `prompt-injection-shaped content`")
		case "safety:unsafe":
			constraints = append(constraints, "secret-like or prompt-injection-shaped content denied: `unsafe content`")
		case "list:strings":
			constraints = append(constraints, "list of strings (or one comma-separated string); entries trimmed, empty entries dropped; key omitted when the list is empty")
		default:
			return "", fmt.Errorf("capability spec %q field %q: validator %q has no payload-contract rendering (fail-closed)", specName, f.Name, v.ID)
		}
	}
	detail := "—"
	if len(constraints) > 0 {
		detail = strings.Join(constraints, "; ")
	}
	return "| `" + f.Name + "` | " + requirement + " | " + detail + " |", nil
}

// payloadSkeleton emits the example --payload object: every declared field in declaration order,
// "..." placeholders, list fields as ["..."]. Field names come from the spec, so a rename changes
// the projected invocation too.
func payloadSkeleton(spec capability.CapabilitySpec) string {
	parts := make([]string, 0, len(spec.Fields))
	for _, f := range spec.Fields {
		if isListField(f) {
			parts = append(parts, `"`+f.Name+`":["..."]`)
		} else {
			parts = append(parts, `"`+f.Name+`":"..."`)
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// enumValues returns the field's enum values in spec order, or nil when the field has no enum
// validator.
func enumValues(f capability.FieldSpec) []string {
	for _, v := range f.Validators {
		if v.ID == "enum" {
			return strings.Split(v.Params["values"], "|")
		}
	}
	return nil
}

func isListField(f capability.FieldSpec) bool {
	return len(f.Validators) == 1 && f.Validators[0].ID == "list:strings"
}

func stringInSlice(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// canonicalSkillContent reads a loop skill's canonical SKILL.md and expands the payload-contract
// marker into the spec-generated section. Skills without the marker project byte-identically to
// their canonical asset; a marker whose contract cannot render fails the install closed — a
// SKILL.md must never project with a literal marker where its payload mechanics belong.
func (c projectorCore) canonicalSkillContent(loop manifest.LoopManifest, skill string) ([]byte, error) {
	content, err := fs.ReadFile(c.assets(), c.loopAsset(loop, skill))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", skill, err)
	}
	if !bytes.Contains(content, []byte(payloadContractMarker)) {
		return content, nil
	}
	section, err := RenderPayloadContract(c.assets(), loop.Name, skillID(skill))
	if err != nil {
		return nil, fmt.Errorf("render payload contract for %s/%s: %w", loop.Name, skillID(skill), err)
	}
	return bytes.ReplaceAll(content, []byte(payloadContractMarker), []byte(section)), nil
}
