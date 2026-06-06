package coreengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
	"github.com/mnemon-dev/mnemon/harness/core/server"
)

// TestLifecycleApplyVisibleViaServerStore is the RED for P1: a governed memory entry applied
// through the lifecycle/app Agent Surface (coreengine.AdmitCreate) MUST be visible when a
// host-agent surface pulls the scoped projection from the canonical harness control store
// (server.DefaultStorePath).
//
// It fails today because the two surfaces own DISJOINT kernel stores: coreengine writes
// <harnessDir>/control/governed.db while the server defaults to server.DefaultStorePath
// (.mnemon/control/server.db). P1.1 unifies that default onto the harness control store and P1
// makes both surfaces share one runtime/store — at which point this turns green. It is pinned
// (t.Skip) until then so the tree never commits red; remove the skip in P1.1.
func TestLifecycleApplyVisibleViaServerStore(t *testing.T) {
	t.Skip("RED for P1.1: unifies the split control store onto server.DefaultStorePath; remove this skip when the default points at the harness control store")

	root := t.TempDir()
	harnessDir := filepath.Join(root, ".mnemon", "harness")

	// lifecycle/app Agent Surface applies a governed memory entry.
	eng := New(harnessDir, seqGen(), fixedNow())
	res, err := eng.AdmitCreate("apply-1", "memory", "m1", map[string]any{"summary": "s", "content": "governed"})
	if err != nil {
		t.Fatalf("AdmitCreate: %v", err)
	}
	if !res.Accepted {
		t.Fatalf("AdmitCreate must be accepted; got %+v", res)
	}

	// host-agent surface pulls the scoped projection from the canonical server store.
	serverStore := filepath.Join(root, server.DefaultStorePath)
	if err := os.MkdirAll(filepath.Dir(serverStore), 0o755); err != nil {
		t.Fatalf("mkdir server store dir: %v", err)
	}
	st, err := kernel.OpenStore(serverStore)
	if err != nil {
		t.Fatalf("open server store: %v", err)
	}
	defer st.Close()

	principal := contract.ActorID("codex@project")
	ref := contract.ResourceRef{Kind: "memory", ID: "m1"}
	subs := map[contract.ActorID]contract.Subscription{principal: {Actor: principal, Refs: []contract.ResourceRef{ref}}}
	k := kernel.NewKernel(st, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{})
	cs := server.New(st, k, rule.NewRuleSet(), subs,
		contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict},
		seqGen(), fixedNow())

	proj, err := cs.PullProjection(principal, contract.Subscription{Actor: principal})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	// ScopedView always materializes one ResourceVersion per subscribed ref, so the store-split
	// symptom is the resource being ABSENT (version 0) in the server store while it is canonical
	// (v1) in coreengine's store — not an empty slice.
	var ver contract.Version
	for _, rv := range proj.Resources {
		if rv.Ref == ref {
			ver = rv.Version
		}
	}
	if ver == 0 {
		t.Fatalf("store split: lifecycle apply wrote m1 to %q but host-agent pull from %q sees m1 @v0 (absent) — the two surfaces own disjoint kernel stores (P0 mismatch; P1 must unify the store)", eng.storePath, serverStore)
	}
}
