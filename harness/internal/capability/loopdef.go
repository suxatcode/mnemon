package capability

import "fmt"

// validateSpecDraft is the body of the validate:capability-spec-draft validator (the D-loop's loopdef
// payload check, P3e): it parses the serialized draft, refuses a draft that would recurse, validates
// the draft COMPILES (FromSpec is pure — it validates and returns a Capability that the caller
// discards, so calling it is validate-only and registers nothing), and runs the SAME untrusted-text
// scan + identifier lock the external loader applies (I15 — a proposed event model is untrusted input).
//
// The single-layer recursion guard is explicit here, NOT in FromSpec: FromSpec accepts any catalogued
// validator id, so a draft naming validate:capability-spec-draft on one of its own fields would pass
// FromSpec and then, once materialized, re-enter this validator. The guard refuses that draft (and a
// draft that is itself a loopdef) up front.
func validateSpecDraft(raw string) error {
	draft, err := decodeSpec([]byte(raw))
	if err != nil {
		return fmt.Errorf("invalid spec draft: %v", err)
	}
	if draft.ResourceKind == "loopdef" || draft.Name == "loopdef" {
		return fmt.Errorf("a loopdef draft may not itself define a loopdef")
	}
	for _, f := range draft.Fields {
		for _, v := range f.Validators {
			if v.ID == "validate:capability-spec-draft" {
				return fmt.Errorf("a loopdef draft may not nest a capability-spec-draft validator")
			}
		}
	}
	if _, err := FromSpec(draft); err != nil {
		return fmt.Errorf("spec draft does not compile: %v", err)
	}
	if err := scanExternalSpecText(draft); err != nil {
		return err
	}
	if err := checkExternalSpecIdentifiers(draft); err != nil {
		return err
	}
	return nil
}
