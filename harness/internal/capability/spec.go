package capability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
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
	// Event-type grammar lock (capability-spec v2 §Grammar): the platform's event types are a
	// CLOSED table of forms over the spec's family segment (eventTypeGrammar). A spec may DECLARE
	// only the two declarable forms — observed_type = <kind>.write_candidate.observed and
	// proposed_type = <kind>.write.proposed — each validated for EQUALITY against the form
	// instantiated with the spec's OWN family, so the event family is bound to the kind, never an
	// open parameter. Without this, a free-form proposed_type compiles, its rule fires, the bridge
	// mints the proposal as a trusted event, and the reconciler (which consumes ONLY *.proposed)
	// silently skips the canonical write: bootable but irreducible. The name doubles as the
	// family segment, so it must use the intake type charset (lowercase, digits, underscore).
	if !specNamePattern.MatchString(spec.Name) {
		return Capability{}, fmt.Errorf("capability spec %q: name must match %s (it is the event-family segment)", spec.Name, specNamePattern.String())
	}
	// Reservation: the system-derived forms (e.g. <kind>.remote_commit.observed, the sync-import
	// observation the platform mints) are NEVER spec-declarable — reject them before the equality
	// check so the error names the real reason, not a generic grammar miss.
	for _, decl := range []struct{ role, val string }{{"observed_type", spec.ObservedType}, {"proposed_type", spec.ProposedType}} {
		for _, form := range eventTypeGrammar {
			if !form.declarable && decl.val == spec.Name+form.suffix {
				return Capability{}, fmt.Errorf("capability spec %q: %s %q is a system-derived form, not spec-declarable", spec.Name, decl.role, decl.val)
			}
		}
	}
	if want := spec.Name + eventTypeObservedSuffix; spec.ObservedType != want {
		return Capability{}, fmt.Errorf("capability spec %q: observed_type %q must be %q (frozen type grammar)", spec.Name, spec.ObservedType, want)
	}
	if want := spec.Name + eventTypeProposedSuffix; spec.ProposedType != want {
		return Capability{}, fmt.Errorf("capability spec %q: proposed_type %q must be %q (frozen type grammar; the reconciler consumes only *.proposed)", spec.Name, spec.ProposedType, want)
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

// LoadSpec reads capabilities/<name>.json from fsys and strictly decodes it into its DATA form,
// for consumers that need the spec itself rather than the compiled Capability (e.g. the SKILL
// payload-contract generator). It goes through decodeSpec — the one fail-closed decode path —
// so there is no second, weaker decoding scheme to drift from it.
func LoadSpec(fsys fs.FS, name string) (CapabilitySpec, error) {
	raw, err := fs.ReadFile(fsys, path.Join("capabilities", name+".json"))
	if err != nil {
		return CapabilitySpec{}, fmt.Errorf("read capability spec %s: %w", name, err)
	}
	spec, err := decodeSpec(raw)
	if err != nil {
		return CapabilitySpec{}, fmt.Errorf("parse capability spec %s: %w", name, err)
	}
	return spec, nil
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
	// Exactly ONE JSON value: Decoder.Decode reads the first value and would silently ignore
	// anything after it ({spec}{garbage} would pass) — LOOSER than the frozen fail-closed
	// contract allows. Require io.EOF on a second read.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return CapabilitySpec{}, fmt.Errorf("trailing data after capability spec (want a single JSON object)")
	}
	return spec, nil
}

// specNamePattern pins capability names to the intake event-type segment charset (server-side
// validateObservedType allows [a-z0-9._]) — a name is the event-family segment by frozen grammar.
var specNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// eventTypeGrammar is the CLOSED table of event-type forms the platform recognises, each a suffix
// over a capability's family segment (= its kind). `declarable` forms are what a capability author
// may write in a spec (observed_type / proposed_type), validated for equality against the family;
// non-declarable forms are SYSTEM-DERIVED — the platform mints them and FromSpec rejects any spec
// that tries to declare one. New event families are added here (a table row), not by reshaping the
// compile path — the G7 extension point. The sync-import observation form is wired in PD6.
type eventTypeForm struct {
	suffix     string
	declarable bool
}

var eventTypeGrammar = []eventTypeForm{
	{suffix: eventTypeObservedSuffix, declarable: true},
	{suffix: eventTypeProposedSuffix, declarable: true},
	{suffix: ".remote_commit.observed", declarable: false}, // sync-import observation (system-derived; PD6)
}

const (
	eventTypeObservedSuffix = ".write_candidate.observed"
	eventTypeProposedSuffix = ".write.proposed"
)

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
