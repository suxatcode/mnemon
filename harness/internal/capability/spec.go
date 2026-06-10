package capability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

// CapabilitySpec is the DATA form of a built-in capability: what a capability author declares in
// assets/capabilities/<name>.json. FromSpec compiles it against the two CLOSED catalogs
// (validators, renders) — a spec can only SELECT compiled members, never define behavior (A3/I8);
// anything unknown fails closed.
type CapabilitySpec struct {
	SchemaVersion int         `json:"schema_version"` // capability spec v1
	Name          string      `json:"name"`
	ObservedType  string      `json:"observed_type"`
	ProposedType  string      `json:"proposed_type"`
	ResourceKind  string      `json:"resource_kind"`
	ItemsField    string      `json:"items_field"`
	Fields        []FieldSpec `json:"fields"`
	Render        RenderSpec  `json:"render"`
}

type FieldSpec struct {
	Name       string         `json:"name"`
	Validators []ValidatorRef `json:"validators,omitempty"`
}

type ValidatorRef struct {
	ID     string            `json:"id"`
	Params map[string]string `json:"params,omitempty"`
}

type RenderSpec struct {
	Content *ContentRender    `json:"content,omitempty"` // nil = no rendered content header
	Static  map[string]string `json:"static,omitempty"`  // literal header fields
}

type ContentRender struct {
	Member string            `json:"member"`
	Params map[string]string `json:"params,omitempty"`
}

// FromSpec compiles a CapabilitySpec into a Capability, fail-closed on everything the spec gets
// wrong: unknown/missing core fields, a resource kind outside contract.KindCatalog, duplicate
// field names, unknown validator/render members, bad or extra member params, forward
// default-from references, list:strings sharing a field with other validators, and render keys
// colliding with the reserved items/updated_by keys.
//
// The compiled Decode contract (parity-frozen, capability spec v1):
//   - ONLY declared fields are processed; payload keys outside the declared set NEVER enter the
//     Item (no leakage into governed state).
//   - For each string field, in declaration order: raw = strings.TrimSpace(stringField(payload,
//     name)); validators run in declared order against the processed value, first error rejects;
//     the processed (trimmed/defaulted) value is what lands in the Item — and EVERY declared
//     string field emits its key (possibly ""), matching the handwritten decoders.
//   - list:strings is the one exception: it uses stringSliceField's full semantics ([]string /
//     []any dropping non-strings / comma-separated string; trimmed, empties compacted) and OMITS
//     the key when the list is empty.
//   - Deny messages are protocol surface: "<name> candidate denied: <member message>".
func FromSpec(spec CapabilitySpec) (Capability, error) {
	if spec.SchemaVersion != 1 {
		return Capability{}, fmt.Errorf("capability spec %q: schema_version %d unsupported (want 1)", spec.Name, spec.SchemaVersion)
	}
	for _, req := range []struct{ name, v string }{
		{"name", spec.Name}, {"observed_type", spec.ObservedType}, {"proposed_type", spec.ProposedType},
		{"resource_kind", spec.ResourceKind}, {"items_field", spec.ItemsField},
	} {
		if strings.TrimSpace(req.v) == "" {
			return Capability{}, fmt.Errorf("capability spec %q: missing %s", spec.Name, req.name)
		}
	}
	if !contract.KindCatalog[contract.ResourceKind(spec.ResourceKind)] {
		return Capability{}, fmt.Errorf("capability spec %q: resource_kind %q not in KindCatalog (fail-closed; register it in contract.KindCatalog + kernel.DefaultSchemaGuard first)", spec.Name, spec.ResourceKind)
	}
	declared := map[string]bool{}
	for _, f := range spec.Fields {
		if strings.TrimSpace(f.Name) == "" {
			return Capability{}, fmt.Errorf("capability spec %q: field with empty name", spec.Name)
		}
		if declared[f.Name] {
			return Capability{}, fmt.Errorf("capability spec %q: duplicate field %q", spec.Name, f.Name)
		}
		isList := false
		for _, v := range f.Validators {
			schema, ok := validatorCatalog[v.ID]
			if !ok {
				return Capability{}, fmt.Errorf("capability spec %q field %q: unknown validator %q (fail-closed)", spec.Name, f.Name, v.ID)
			}
			if err := checkParams(v.Params, schema); err != nil {
				return Capability{}, fmt.Errorf("capability spec %q field %q validator %q: %w", spec.Name, f.Name, v.ID, err)
			}
			switch v.ID {
			case "required":
				if s := v.Params["missing_style"]; s != "empty" && s != "missing" {
					return Capability{}, fmt.Errorf("capability spec %q field %q: missing_style %q must be empty|missing", spec.Name, f.Name, s)
				}
			case "default-from":
				if !declared[v.Params["field"]] {
					return Capability{}, fmt.Errorf("capability spec %q field %q: default-from %q must reference a previously declared field", spec.Name, f.Name, v.Params["field"])
				}
			case "list:strings":
				isList = true
			}
		}
		if isList && len(f.Validators) != 1 {
			return Capability{}, fmt.Errorf("capability spec %q field %q: list:strings must be the field's only validator", spec.Name, f.Name)
		}
		declared[f.Name] = true
	}

	// Render: member + params + reserved-key collision guards.
	produced := map[string]bool{}
	for k := range spec.Render.Static {
		produced[k] = true
	}
	if c := spec.Render.Content; c != nil {
		schema, ok := renderCatalog[c.Member]
		if !ok {
			return Capability{}, fmt.Errorf("capability spec %q: unknown render %q (fail-closed)", spec.Name, c.Member)
		}
		if err := checkParams(c.Params, schema); err != nil {
			return Capability{}, fmt.Errorf("capability spec %q render %q: %w", spec.Name, c.Member, err)
		}
		if c.Member == "bullet-list" && !declared[c.Params["field"]] {
			return Capability{}, fmt.Errorf("capability spec %q render bullet-list: field %q not declared", spec.Name, c.Params["field"])
		}
		if produced["content"] {
			return Capability{}, fmt.Errorf("capability spec %q: render static and content slot both produce \"content\"", spec.Name)
		}
		produced["content"] = true
	}
	for k := range produced {
		if k == spec.ItemsField || k == "updated_by" {
			return Capability{}, fmt.Errorf("capability spec %q: render key %q collides with a reserved resource key", spec.Name, k)
		}
	}

	return Capability{
		Name:         spec.Name,
		ObservedType: spec.ObservedType,
		ProposedType: spec.ProposedType,
		ResourceKind: contract.ResourceKind(spec.ResourceKind),
		ItemsField:   spec.ItemsField,
		Decode:       compileDecode(spec),
		Header:       compileHeader(spec),
	}, nil
}

// decodeSpec is the ONE way a CapabilitySpec is read from JSON: DisallowUnknownFields makes the
// frozen protocol surface fail-closed at the SYNTAX level too — an unknown key anywhere (top
// level, field object, validator object, render object) rejects the spec instead of silently
// compiling a typo into default behavior. Production loading and the golden tests share it.
func decodeSpec(raw []byte) (CapabilitySpec, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var spec CapabilitySpec
	if err := dec.Decode(&spec); err != nil {
		return CapabilitySpec{}, err
	}
	return spec, nil
}

type paramSchema struct{ required, optional []string }

func checkParams(params map[string]string, schema paramSchema) error {
	allowed := map[string]bool{}
	for _, k := range schema.required {
		if strings.TrimSpace(params[k]) == "" {
			return fmt.Errorf("missing param %q", k)
		}
		allowed[k] = true
	}
	for _, k := range schema.optional {
		allowed[k] = true
	}
	for k := range params {
		if !allowed[k] {
			return fmt.Errorf("unknown param %q (fail-closed)", k)
		}
	}
	return nil
}
