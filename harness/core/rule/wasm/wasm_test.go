package wasm

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func evWith(payload map[string]any) contract.Event {
	return contract.Event{Type: "memory.observed", Payload: payload}
}

// S12: a real wazero-executed .wasm makes a real input-dependent decision (deny without evidence, propose
// with it).
func TestWasmRuleEvaluates(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx, readBytes(t, "testdata/rule_allow_if_evidence.wasm"), Limits{Timeout: 50 * time.Millisecond, MemPages: 16})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if d, err := r.Evaluate(rule.RuleInput{Event: evWith(nil)}); err != nil || d.Verdict != contract.VerdictDeny {
		t.Fatalf("missing evidence -> deny; got %q err=%v", d.Verdict, err)
	}
	if d, err := r.Evaluate(rule.RuleInput{Event: evWith(map[string]any{"evidence": "x"})}); err != nil || d.Verdict != contract.VerdictPropose {
		t.Fatalf("evidence -> propose; got %q err=%v", d.Verdict, err)
	}
}

// S12: a runaway module is killed by the per-call deadline (sys.ExitError-wrapped error), never a hang.
func TestWasmRunawayIsKilledByDeadline(t *testing.T) {
	ctx := context.Background()
	r, err := New(ctx, readBytes(t, "testdata/loop.wasm"), Limits{Timeout: 5 * time.Millisecond, MemPages: 16})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, e := r.Evaluate(rule.RuleInput{Event: evWith(nil)}); done <- e }()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("a runaway module must return a deadline error, not succeed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a runaway module must be killed by the deadline, not hang")
	}
}

// S12: the module imports only env.read_state_view -> it instantiates with NO wasi registered.
func TestWasmInstantiatesWithoutWASI(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx, readBytes(t, "testdata/rule_allow_if_evidence.wasm"), Limits{Timeout: 50 * time.Millisecond, MemPages: 16}); err != nil {
		t.Fatalf("a module importing only env.read_state_view must instantiate without WASI: %v", err)
	}
}
