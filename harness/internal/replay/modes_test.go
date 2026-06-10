package replay

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

// replay 必须在与 live server 完全相同的模式下归约,否则重放可接受 live 已拒绝的冲突
// (历史缺陷:replay 硬编码 Rebase,server 默认 Reject)。单一来源杀类。
func TestReplayModesMatchServerDefault(t *testing.T) {
	want := contract.DefaultModes()
	if canonicalModes != want {
		t.Fatalf("replay modes %+v must equal contract.DefaultModes() %+v", canonicalModes, want)
	}
	if want.Conflict != contract.ConflictReject {
		t.Fatalf("the platform default conflict mode is reject, got %q", want.Conflict)
	}
}

// 钉住新默认下的唯一行为位移:sampleEvents 的 stale p3 在 Reject 默认下是 Rejected
// (旧 replay 私有 Rebase 下曾是 Deferred/rebase)。
func TestStaleProposalRejectsUnderDefaultModes(t *testing.T) {
	events := sampleEvents
	_, live := liveDecisions(t, events)
	var found bool
	for _, d := range live {
		if len(d.Conflicts) > 0 {
			found = true
			if d.Status != contract.Rejected {
				t.Fatalf("the stale proposal must be Rejected under contract.DefaultModes(), got %v (next %q)", d.Status, d.NextAction)
			}
		}
	}
	if !found {
		t.Fatal("sampleEvents must contain the stale-conflict decision this test pins")
	}
}
