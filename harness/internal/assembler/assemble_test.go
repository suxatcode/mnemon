package assembler

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// A 3rd capability (note) stands up end-to-end through config + the generic kind alone — no new rule
// code: Assemble compiles the config into a runtime config whose note rule admits a note candidate
// through the channel -> tick -> kernel -> projection.
func TestAssembleAdmitsConfiguredNoteCapabilityEndToEnd(t *testing.T) {
	ref := contract.ResourceRef{Kind: "note", ID: "project"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	binding.AllowedObservedTypes = []string{"note.write_candidate.observed"}

	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"note": {Enabled: true, ResourceRef: "note/project", RuleRef: "native:note"},
	}}
	rc, err := Assemble(cfg, []channel.ChannelBinding{binding}, nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "g.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "n1",
		Event:      contract.Event{Type: "note.write_candidate.observed", Payload: map[string]any{"text": "remember the assembler"}},
	}); err != nil {
		t.Fatalf("ingest note: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	v, fields, err := rt.Resource(ref)
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	if v == 0 {
		t.Fatal("the configured note capability must admit a candidate (resource not created)")
	}
	if content, _ := fields["content"].(string); !strings.Contains(content, "remember the assembler") {
		t.Fatalf("note content missing the candidate: %q", content)
	}
}

// Stage-5: Assemble selects from the PROVIDED catalog — a capability that exists only in an
// external package (goal) resolves when the resolved catalog is passed, and fails closed when the
// caller passes nil (nil = capability.Builtins, the backward-compatible seam).
func TestAssembleResolvesFromProvidedCatalog(t *testing.T) {
	goalSpec := capability.CapabilitySpec{
		SchemaVersion: 1, Name: "goal",
		ObservedType: "goal.write_candidate.observed", ProposedType: "goal.write.proposed",
		ResourceKind: "goal", ItemsField: "items",
		Fields: []capability.FieldSpec{{Name: "statement", Validators: []capability.ValidatorRef{
			{ID: "required", Params: map[string]string{"missing_style": "empty"}},
		}}},
		Render: capability.RenderSpec{
			Content: &capability.ContentRender{Member: "bullet-list", Params: map[string]string{"title": "# Goals", "field": "statement"}},
			Static:  map[string]string{"statement": "project"},
		},
	}
	goalCap, err := capability.FromSpec(goalSpec)
	if err != nil {
		t.Fatalf("compile goal spec: %v", err)
	}
	catalog := map[string]capability.Capability{"goal": goalCap}
	for id, c := range capability.Builtins {
		catalog[id] = c
	}

	ref := contract.ResourceRef{Kind: "goal", ID: "project"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	binding.AllowedObservedTypes = []string{"goal.write_candidate.observed"}
	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"goal": {Enabled: true, ResourceRef: "goal/project", RuleRef: "native:goal"},
	}}

	if _, err := Assemble(cfg, []channel.ChannelBinding{binding}, nil); err == nil {
		t.Fatal("native:goal must fail closed against the nil (Builtins) catalog")
	}
	rc, err := Assemble(cfg, []channel.ChannelBinding{binding}, catalog)
	if err != nil {
		t.Fatalf("assemble with external-merged catalog: %v", err)
	}

	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "g.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()
	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "g1",
		Event:      contract.Event{Type: "goal.write_candidate.observed", Payload: map[string]any{"statement": "ship stage five"}},
	}); err != nil {
		t.Fatalf("ingest goal: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	v, fields, err := rt.Resource(ref)
	if err != nil || v == 0 {
		t.Fatalf("the catalog-selected goal capability must admit (v=%d err=%v)", v, err)
	}
	if content, _ := fields["content"].(string); !strings.Contains(content, "ship stage five") {
		t.Fatalf("goal content missing the candidate: %q", content)
	}
}

func TestAssembleFailsClosedOnUnknownCapability(t *testing.T) {
	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"bogus": {Enabled: true, ResourceRef: "bogus/project", RuleRef: "native:bogus"},
	}}
	if _, err := Assemble(cfg, nil, nil); err == nil {
		t.Fatal("an unknown capability rule_ref must fail closed")
	}
}

// A binding scoped to a non-default ref of the capability's kind must get a rule targeting ITS ref
// (parity with the production memoryRefForBinding fallback), not the config-pinned default.
func TestAssembleDerivesRefFromBindingScope(t *testing.T) {
	teamRef := contract.ResourceRef{Kind: "memory", ID: "team"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{teamRef})
	binding.AllowedObservedTypes = []string{"memory.write_candidate.observed"}

	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"memory": {Enabled: true, ResourceRef: "memory/project", RuleRef: "native:memory"},
	}}
	rc, err := Assemble(cfg, []channel.ChannelBinding{binding}, nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "g.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "m1",
		Event:      contract.Event{Type: "memory.write_candidate.observed", Payload: map[string]any{"content": "team fact", "source": "s", "confidence": "high"}},
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if v, _, err := rt.Resource(teamRef); err != nil || v == 0 {
		t.Fatalf("write must land on the binding's scoped ref memory/team (v=%d err=%v)", v, err)
	}
	if v, _, _ := rt.Resource(contract.ResourceRef{Kind: "memory", ID: "project"}); v != 0 {
		t.Fatal("the config default memory/project must NOT be written for a team-scoped binding")
	}
}

// A host-agent binding with observe + observed-type but EMPTY SubscriptionScope must produce no rule
// and no kernel authority (parity with the app builders' skip; an unscoped binding could never pull
// what it writes).
func TestAssembleSkipsUnscopedBinding(t *testing.T) {
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", nil)
	binding.AllowedObservedTypes = []string{"memory.write_candidate.observed"}

	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"memory": {Enabled: true, ResourceRef: "memory/project", RuleRef: "native:memory"},
	}}
	rc, err := Assemble(cfg, []channel.ChannelBinding{binding}, nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if got := len(rc.Authority.Allow["codex@project"]); got != 0 {
		t.Fatalf("unscoped binding must get no kernel authority, got %d kinds", got)
	}

	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "g.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()
	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "m1",
		Event:      contract.Event{Type: "memory.write_candidate.observed", Payload: map[string]any{"content": "x", "source": "s", "confidence": "high"}},
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if v, _, _ := rt.Resource(contract.ResourceRef{Kind: "memory", ID: "project"}); v != 0 {
		t.Fatal("an unscoped binding must not produce a write")
	}
}

// rule_ref 必须携带命名空间前缀:裸 id(如 "memory")在 Assemble 这道生产 seam
// 上 fail-closed —— 为未来的 wasm: 等命名空间立规,与 config.Load 的校验双门一致。
func TestAssembleRejectsBareRuleRef(t *testing.T) {
	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"memory": {Enabled: true, ResourceRef: "memory/project", RuleRef: "memory"}, // 缺 native: 前缀
	}}
	if _, err := Assemble(cfg, nil, nil); err == nil {
		t.Fatal("a bare rule_ref without the native: namespace prefix must fail closed")
	}
}

// 阶段二验收:第四能力 decision 的全部 Go 足迹 = KindCatalog/SchemaGuard 各一行;
// 行为完全来自 assets/capabilities/decision.json(spec 文件)。端到端与 note 同构。
func TestAssembleAdmitsDecisionCapabilityEndToEnd(t *testing.T) {
	ref := contract.ResourceRef{Kind: "decision", ID: "project"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	binding.AllowedObservedTypes = []string{"decision.write_candidate.observed"}

	cfg := config.File{Capabilities: map[string]config.CapabilityConfig{
		"decision": {Enabled: true, ResourceRef: "decision/project", RuleRef: "native:decision"},
	}}
	rc, err := Assemble(cfg, []channel.ChannelBinding{binding}, nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	rt, err := runtime.OpenRuntime(filepath.Join(t.TempDir(), "g.db"), rc)
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()
	if _, _, err := rt.API().Ingest("codex@project", contract.ObservationEnvelope{
		ExternalID: "d1",
		Event:      contract.Event{Type: "decision.write_candidate.observed", Payload: map[string]any{"text": "adopt the spec catalogs"}},
	}); err != nil {
		t.Fatalf("ingest decision: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	v, fields, err := rt.Resource(ref)
	if err != nil || v == 0 {
		t.Fatalf("decision capability must admit (v=%d err=%v)", v, err)
	}
	if content, _ := fields["content"].(string); !strings.Contains(content, "adopt the spec catalogs") {
		t.Fatalf("decision content missing the candidate: %q", content)
	}
}

// Header⊇SchemaGuard 锁步:每个内置能力的渲染产物必须覆盖其 kind 的全部必填字段——
// 否则 spec 文件能声明一个 kernel 永远拒绝的能力(装配期可发现的缺陷不留到运行期)。
func TestBuiltinHeadersSatisfySchemaGuard(t *testing.T) {
	guard := kernel.DefaultSchemaGuard()
	for id, cap := range capability.Builtins {
		item, err := cap.Decode(minimalAcceptPayload(id))
		if err != nil {
			t.Fatalf("%s: decode minimal accept: %v", id, err)
		}
		fields := map[string]any{cap.ItemsField: []capability.Item{item}, "updated_by": "x"}
		for k, v := range cap.Header([]capability.Item{item}) {
			fields[k] = v
		}
		if err := guard.Validate(cap.ResourceKind, fields); err != nil {
			t.Fatalf("%s: rendered fields must satisfy SchemaGuard: %v", id, err)
		}
	}
}

func minimalAcceptPayload(id string) map[string]any {
	switch id {
	case "memory":
		return map[string]any{"content": "x", "source": "s", "confidence": "high"}
	case "skill":
		return map[string]any{"skill_id": "x-skill", "source": "s", "confidence": "high"}
	default:
		return map[string]any{"text": "x"}
	}
}
