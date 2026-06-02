package reconcile

import (
	"testing"

	"github.com/mnemon-dev/mnemon/core/contract"
)

// #2: the authz catalog must advertise only modes the kernel actually delivers. permissive/audit_only/
// dry_run are not implemented (Apply enforces rules unconditionally = strict), so they must NOT be
// selectable via config — otherwise the catalog promises behavior it cannot deliver, and selecting
// dry_run would still commit. (Reserved like `serializable` until they have real, distinct teeth.)
func TestUnimplementedAuthzModesNotSelectable(t *testing.T) {
	for _, bad := range []string{contract.AuthzPermissive, contract.AuthzAuditOnly, contract.AuthzDryRun} {
		if _, err := ResolveModes(Config{Conflict: contract.ConflictReject, Isolation: contract.IsolationWriteCAS, Authz: bad}); err == nil {
			t.Fatalf("authz mode %q must NOT be selectable until implemented", bad)
		}
	}
	if _, err := ResolveModes(Config{Conflict: contract.ConflictReject, Isolation: contract.IsolationWriteCAS, Authz: contract.AuthzStrict}); err != nil {
		t.Fatalf("strict must remain selectable: %v", err)
	}
}
