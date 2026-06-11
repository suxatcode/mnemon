package hostsurface

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

var (
	hookgenHosts = []string{"claude-code", "codex"}
	hookgenLoops = []string{"memory", "skill"}
)

// hookGoldens pins the sha256 of every generated hook, (host/loop/timing) -> digest. They were
// minted at the migration commit where generated == legacy held byte-for-byte with an EMPTY patch
// table (the stage-3 acceptance bar), so the goldens ARE the legacy bytes' fingerprints. Any
// generator/template/intents/mechanics change shows up here as a reviewable diff; a deliberate
// behavior change (e.g. the claude compact escape fix) updates the golden in the same commit that
// records the decision.
var hookGoldens = map[string]string{
	"claude-code/memory/compact": "0281afc8283922df9ee4b7a1fabf9910776079c66b107f3d2d8179337ce3eec1",
	"claude-code/memory/nudge":   "870336ca55cc85bb891f3abdfe6477bc851d4b7215476ffb4d70427c6be15c59",
	"claude-code/memory/prime":   "7d3f4fc0c0438371a5d4d5f1b9451f4b84bdb8adede836d2e52f5ceb5a1b9b3e",
	"claude-code/memory/remind":  "6b755a8e8325abf9e52442402a04d6f3e9da77dfe40762ea187ff170be95b4d0",
	"claude-code/skill/compact":  "dd0c690b9e8b12b0bce3ea35c2fd0e7396d6dc3752d2e74afcb46b7340914aa5",
	"claude-code/skill/nudge":    "4177fa0388447132f26e3bee2b33254272fd164c8f0e7a95b00ca9d35576b6fb",
	"claude-code/skill/prime":    "c7309e602979940c0d0aa62e75ceb0bf15ddd36d37e05f5d83e50caf4f818db1",
	"claude-code/skill/remind":   "39508d6cc8ec74307b8f8b65719a4e48f36f87e741aeaaf8f87166484c466a9e",
	"codex/memory/compact":       "d7021b8ce2bf4a00bab6946254a93e5fe4512ba0fb7be7b87955ef79aab76b7e",
	"codex/memory/nudge":         "6673d442815e416ebdebac471deefadfa5f5340c70c9326a866368030a4aa585",
	"codex/memory/prime":         "7d3f4fc0c0438371a5d4d5f1b9451f4b84bdb8adede836d2e52f5ceb5a1b9b3e",
	"codex/memory/remind":        "de3a08eef56f9406f96d7c961d43215702e83a7974ea0e652accc9e6d2a342c3",
	"codex/skill/compact":        "466b4ccdfef70ec931db795d7885b8da98adc1ab10b63c6a6f44ebfe80fe470f",
	"codex/skill/nudge":          "ad914b0849a1dc2a69c5580f9434b78e7bff15167bc74bf393cacfdd12169844",
	"codex/skill/prime":          "93621a58110dcd1ee6ff97745dcac64e04436a7cb7a32f38c4d7b4df49270d64",
	"codex/skill/remind":         "39508d6cc8ec74307b8f8b65719a4e48f36f87e741aeaaf8f87166484c466a9e",
}

// TestGeneratedHooksMatchGoldens is the standing protocol pin for the generated hook surface
// (successor of the migration-time byte-parity test against the now-retired legacy assets).
func TestGeneratedHooksMatchGoldens(t *testing.T) {
	seen := 0
	for _, host := range []string{"codex", "claude-code"} {
		for _, loop := range []string{"memory", "skill"} {
			for _, timing := range []string{"prime", "remind", "nudge", "compact"} {
				key := host + "/" + loop + "/" + timing
				content, err := RenderHook(loop, host, timing)
				if err != nil {
					t.Fatalf("render %s: %v", key, err)
				}
				sum := sha256.Sum256([]byte(content))
				if got := hex.EncodeToString(sum[:]); got != hookGoldens[key] {
					t.Fatalf("%s: generated content drifted from golden\ngot:  %s\nwant: %s\n--- generated ---\n%s", key, got, hookGoldens[key], content)
				}
				seen++
			}
		}
	}
	if seen != 16 || len(hookGoldens) != 16 {
		t.Fatalf("golden table must cover exactly the 16 hooks (seen %d, table %d)", seen, len(hookGoldens))
	}
}

func TestHookgenDeterministic(t *testing.T) {
	for _, host := range hookgenHosts {
		for _, loop := range hookgenLoops {
			for _, timing := range hookTimings {
				first, err := RenderHook(loop, host, timing)
				if err != nil {
					t.Fatalf("RenderHook(%s, %s, %s) first render: %v", loop, host, timing, err)
				}
				second, err := RenderHook(loop, host, timing)
				if err != nil {
					t.Fatalf("RenderHook(%s, %s, %s) second render: %v", loop, host, timing, err)
				}
				if first != second {
					t.Fatalf("RenderHook(%s, %s, %s) is not deterministic:\n%s", loop, host, timing, describeDivergence(first, second))
				}
			}
		}
	}
}

// --- fail-closed decode tests: the vocabulary is a CLOSED set, unknown members must error ---

const validMinimalIntents = `{"schema_version":1,"hooks":{"remind":{"response":{"role":"one-liner","text":"hello"}}}}`

func TestDecodeHookIntentsFailClosed(t *testing.T) {
	cases := map[string]string{
		"unknown timing": `{"schema_version":1,"hooks":{"bogus":{"response":{"role":"one-liner","text":"x"}}}}`,
		"unknown gate type": `{"schema_version":1,"hooks":{"nudge":{"gates":[{"type":"wormhole"}],
			"response":{"role":"one-liner","text":"x"}}}}`,
		"unknown section type":  `{"schema_version":1,"hooks":{"prime":{"sections":[{"type":"telemetry"}]}}}`,
		"unknown json field":    `{"schema_version":1,"hooks":{"remind":{"responze":{"role":"one-liner","text":"x"}}}}`,
		"trailing json":         validMinimalIntents + `{}`,
		"wrong schema version":  `{"schema_version":2,"hooks":{"remind":{"response":{"role":"one-liner","text":"x"}}}}`,
		"unknown response role": `{"schema_version":1,"hooks":{"remind":{"response":{"role":"shout","text":"x"}}}}`,
		"unknown threshold metric": `{"schema_version":1,"hooks":{"compact":{"gates":[{"type":"threshold",
			"metric":"entropy","cmp":"gt","limit_env":"X","limit_default":"1"}],
			"response":{"role":"message","text":"x"}}}}`,
		"unknown threshold cmp": `{"schema_version":1,"hooks":{"compact":{"gates":[{"type":"threshold",
			"metric":"usage-event-count","cmp":"lt","file_env":"X","file_default":"y","limit_env":"Z","limit_default":"1"}],
			"response":{"role":"message","text":"x"}}}}`,
		"gate param on wrong type": `{"schema_version":1,"hooks":{"nudge":{"gates":[{"type":"if-input-field",
			"field":"stop_hook_active","marker":"prime"}],"response":{"role":"one-liner","text":"x"}}}}`,
		"select without threshold": `{"schema_version":1,"hooks":{"nudge":{"response":{"role":"message",
			"over":"a","under":"b"}}}}`,
		"shell-active slot text": `{"schema_version":1,"hooks":{"remind":{"response":{"role":"one-liner",
			"text":"hi $(rm -rf /)"}}}}`,
	}
	for name, raw := range cases {
		if _, err := decodeHookIntents([]byte(raw)); err == nil {
			t.Errorf("%s: decodeHookIntents accepted invalid input", name)
		}
	}
	if _, err := decodeHookIntents([]byte(validMinimalIntents)); err != nil {
		t.Errorf("valid minimal intents rejected: %v", err)
	}
}

func TestDecodeHostMechanicsFailClosed(t *testing.T) {
	wrap := func(mechanics string) []byte {
		return []byte(`{"schema_version":2,"name":"x","mechanics":` + mechanics + `}`)
	}
	valid := `{"stdin_read":{"default":"tolerant"},"dialect":{"default":"plain"},"json_escape":false}`
	cases := map[string][]byte{
		"missing mechanics":   []byte(`{"schema_version":2,"name":"x"}`),
		"unknown stdin idiom": wrap(`{"stdin_read":{"default":"buffered"},"dialect":{"default":"plain"},"json_escape":false}`),
		"unknown dialect":     wrap(`{"stdin_read":{"default":"strict"},"dialect":{"default":"yaml"},"json_escape":false}`),
		"unknown json field":  wrap(`{"stdin_read":{"default":"strict"},"dialect":{"default":"plain"},"json_escape":false,"shell":"zsh"}`),
		"trailing json":       wrap(valid + `{}`),
		"unknown override timing": wrap(`{"stdin_read":{"default":"strict","overrides":{"memory":{"boot":"tolerant"}}},
			"dialect":{"default":"plain"},"json_escape":false}`),
		"unknown override value": wrap(`{"stdin_read":{"default":"strict","overrides":{"memory":{"nudge":"buffered"}}},
			"dialect":{"default":"plain"},"json_escape":false}`),
		"unknown wording slot": wrap(`{"stdin_read":{"default":"strict"},"dialect":{"default":"plain"},"json_escape":false,
			"wording_overrides":{"memory":{"remind":{"texxt":"hi"}}}}`),
		"unknown marker timing": wrap(`{"stdin_read":{"default":"strict"},"dialect":{"default":"plain"},"json_escape":false,
			"marker_overrides":{"memory":{"boot":false}}}`),
	}
	for name, raw := range cases {
		if _, err := decodeHostMechanics(raw); err == nil {
			t.Errorf("%s: decodeHostMechanics accepted invalid input", name)
		}
	}
	if _, err := decodeHostMechanics(wrap(valid)); err != nil {
		t.Errorf("valid minimal mechanics rejected: %v", err)
	}
}

// RenderHook itself must fail closed on unknown coordinates: a misspelled timing or loop cannot
// fall back to an empty hook.
func TestRenderHookFailClosed(t *testing.T) {
	if _, err := RenderHook("memory", "codex", "boot"); err == nil {
		t.Error("unknown timing accepted")
	}
	if _, err := RenderHook("nonexistent", "codex", "prime"); err == nil {
		t.Error("unknown loop accepted")
	}
	if _, err := RenderHook("memory", "nonexistent", "prime"); err == nil {
		t.Error("unknown host accepted")
	}
}

// describeDivergence pinpoints the first differing byte between two renders (debug aid).
func describeDivergence(a, b string) string {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			start := i - 40
			if start < 0 {
				start = 0
			}
			return fmt.Sprintf("first divergence at byte %d:\nA: %q\nB: %q", i, a[start:i+20], b[start:i+20])
		}
	}
	return fmt.Sprintf("length divergence: %d vs %d", len(a), len(b))
}
