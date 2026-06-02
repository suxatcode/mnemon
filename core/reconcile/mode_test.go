package reconcile

import "testing"

func TestResolveModesSelectsCatalogOnly(t *testing.T) {
	if _, err := ResolveModes(Config{Conflict: "defer_to_human", Isolation: "projection_read_set", Authz: "strict"}); err != nil {
		t.Fatalf("valid select failed: %v", err)
	}
	// define≠select: a script in ANY field must be REJECTED, never run.
	for _, bad := range []Config{
		{Conflict: "./evil.sh", Isolation: "projection_read_set", Authz: "strict"},
		{Conflict: "reject", Isolation: "./evil.sh", Authz: "strict"},
		{Conflict: "reject", Isolation: "write_cas", Authz: "./evil.sh"},
	} {
		if _, err := ResolveModes(bad); err == nil {
			t.Fatalf("non-catalog mode accepted — SAFETY BREACH: %+v", bad)
		}
	}
}
