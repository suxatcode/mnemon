package capability

import "strings"

// ObservedTypeAndAliases returns a canonical observed event type together with its accepted aliases,
// so the channel admits and the rule handles both during the dotted-naming convergence. The canonical
// form is dot-segmented (e.g. "memory.write_candidate.observed"); the legacy alias is the same type
// with its last dot rendered as an underscore ("memory.write_candidate_observed"). A canonical type
// with no convertible last segment returns just itself.
func ObservedTypeAndAliases(canonical string) []string {
	legacy := legacyUnderscore(canonical)
	if legacy == canonical {
		return []string{canonical}
	}
	return []string{canonical, legacy}
}

func legacyUnderscore(t string) string {
	i := strings.LastIndex(t, ".")
	if i < 0 {
		return t
	}
	return t[:i] + "_" + t[i+1:]
}
