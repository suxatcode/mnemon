package capability

import "testing"

const validDraft = `{"schema_version":1,"name":"widget2","observed_type":"widget2.write_candidate.observed",
"proposed_type":"widget2.write.proposed","resource_kind":"widget2","items_field":"items",
"fields":[{"name":"text","validators":[{"id":"required","params":{"missing_style":"empty"}}]}],
"render":{"content":{"member":"bullet-list","params":{"title":"# W2","field":"text"}}}}`

func TestValidateSpecDraft(t *testing.T) {
	if err := validateSpecDraft(validDraft); err != nil {
		t.Fatalf("a well-formed draft must validate: %v", err)
	}
	if err := validateSpecDraft("not json at all"); err == nil {
		t.Fatal("a non-JSON draft must be rejected")
	}
	// recursion guard: a draft that is itself a loopdef.
	loopdefDraft := `{"schema_version":1,"name":"loopdef2","observed_type":"loopdef2.write_candidate.observed",
"proposed_type":"loopdef2.write.proposed","resource_kind":"loopdef","items_field":"items",
"fields":[{"name":"x","validators":[{"id":"required","params":{"missing_style":"empty"}}]}],
"render":{"content":{"member":"bullet-list","params":{"title":"# X","field":"x"}}},"risk":"high"}`
	if err := validateSpecDraft(loopdefDraft); err == nil {
		t.Fatal("a draft that defines a loopdef must be rejected (recursion guard)")
	}
	// recursion guard: a draft that nests the spec-draft validator.
	nestedDraft := `{"schema_version":1,"name":"nest","observed_type":"nest.write_candidate.observed",
"proposed_type":"nest.write.proposed","resource_kind":"nest","items_field":"items",
"fields":[{"name":"inner","validators":[{"id":"validate:capability-spec-draft"}]}],
"render":{"content":{"member":"bullet-list","params":{"title":"# N","field":"inner"}}}}`
	if err := validateSpecDraft(nestedDraft); err == nil {
		t.Fatal("a draft nesting a spec-draft validator must be rejected (recursion guard)")
	}
	// does not compile: an unknown validator id.
	badDraft := `{"schema_version":1,"name":"bad","observed_type":"bad.write_candidate.observed",
"proposed_type":"bad.write.proposed","resource_kind":"bad","items_field":"items",
"fields":[{"name":"y","validators":[{"id":"no-such-validator"}]}],
"render":{"content":{"member":"bullet-list","params":{"title":"# B","field":"y"}}}}`
	if err := validateSpecDraft(badDraft); err == nil {
		t.Fatal("a draft that fails FromSpec must be rejected")
	}
}

// S4/G2: a loopdef kind must be high-risk — FromSpec rejects a lower tier.
func TestLoopdefMustBeHighRisk(t *testing.T) {
	spec, err := decodeSpec([]byte(`{"schema_version":1,"name":"loopdef","observed_type":"loopdef.write_candidate.observed",
"proposed_type":"loopdef.write.proposed","resource_kind":"loopdef","items_field":"items",
"fields":[{"name":"spec","validators":[{"id":"required","params":{"missing_style":"empty"}}]}],
"render":{"content":{"member":"bullet-list","params":{"title":"# L","field":"spec"}}},"risk":"mid"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := FromSpec(spec); err == nil {
		t.Fatal("a loopdef kind with risk:mid must be rejected (G2 non-overridable)")
	}
}
