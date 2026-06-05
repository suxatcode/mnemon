package rule

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
)

// Manifest describes a candidate (wasm) rule for governed promotion: its identity, the sha256 of its bytes,
// the host capabilities it declares, the event types it handles, and whether it is deterministic.
type Manifest struct {
	ID, Version, SHA256 string
	Capabilities        []string
	Handles             []string
	Deterministic       bool
}

// Registry is the active rule set plus the governed promotion gate (S12). A candidate is admitted ONLY if its
// bytes hash to the manifest, its import section is exactly {env.read_state_view}, and its shadow report is
// clean — changing the rules is itself a governed action, never a free side-channel.
type Registry struct {
	active []Rule
}

func NewRegistry(rules ...Rule) *Registry { return &Registry{active: rules} }

// Active returns the current active rule set.
func (reg *Registry) Active() RuleSet { return NewRuleSet(reg.active...) }

// Promote admits a rule into the active set iff: sha256(wasmBytes) == m.SHA256 (signed/pinned identity), the
// wasm import section is EXACTLY {env.read_state_view} (no WASI, no extra host reach), and report.Clean (the
// shadow produced no divergence the operator did not accept). The active rule is BUILT FROM the verified
// bytes via build (so the rule that goes active is structurally the verified module, not an unrelated
// candidate — build, e.g. wasmrule.New, also re-validates the bytes by instantiation, defense-in-depth). Any
// failure leaves the active set untouched.
func (reg *Registry) Promote(wasmBytes []byte, build func([]byte) (Rule, error), m Manifest, report ShadowReport) error {
	sum := sha256.Sum256(wasmBytes)
	if hex.EncodeToString(sum[:]) != m.SHA256 {
		return fmt.Errorf("promotion: sha256 mismatch (bytes do not match manifest)")
	}
	imports, err := wasmImports(wasmBytes)
	if err != nil {
		return fmt.Errorf("promotion: %w", err)
	}
	if len(imports) != 1 || imports[0] != "env.read_state_view" {
		return fmt.Errorf("promotion: import section must be exactly {env.read_state_view}, got %v", imports)
	}
	if !report.Clean {
		return fmt.Errorf("promotion: shadow report not clean (%d diffs)", report.Diffs)
	}
	r, err := build(wasmBytes)
	if err != nil {
		return fmt.Errorf("promotion: build rule from verified bytes: %w", err)
	}
	reg.active = append(reg.active, r)
	return nil
}

// EdgeSnapshot returns a DENY-ONLY view of a rule set for an untrusted edge (D10): each rule's verdict is
// filtered to {deny,warn}. A propose / enqueue_job / request_evidence / allow becomes an advisory warn with
// the original verdict recorded in the reasons (and any proposal dropped) — an edge may refuse, never author.
func EdgeSnapshot(rs RuleSet) RuleSet {
	wrapped := make([]Rule, 0, len(rs.rules))
	for _, r := range rs.rules {
		wrapped = append(wrapped, edgeRule{inner: r})
	}
	return NewRuleSet(wrapped...)
}

type edgeRule struct{ inner Rule }

func (e edgeRule) ID() string              { return e.inner.ID() }
func (e edgeRule) Actor() contract.ActorID { return e.inner.Actor() }
func (e edgeRule) Emits() string           { return e.inner.Emits() }
func (e edgeRule) Handles(t string) bool   { return e.inner.Handles(t) }
func (e edgeRule) Evaluate(in RuleInput) (contract.RuleDecision, error) {
	d, err := e.inner.Evaluate(in)
	if err != nil {
		return contract.RuleDecision{}, err
	}
	if d.Verdict == contract.VerdictDeny || d.Verdict == contract.VerdictWarn {
		// an edge may refuse/warn but never AUTHOR — strip any proposal/job riding on the verdict.
		return contract.RuleDecision{Verdict: d.Verdict, Reasons: d.Reasons}, nil
	}
	return contract.RuleDecision{
		Verdict: contract.VerdictWarn,
		Reasons: append(d.Reasons, "edge: "+string(d.Verdict)+" downgraded to warn (edge is deny-only)"),
	}, nil
}

// ---- minimal WASM import-section parser (no wazero dependency; rule stays lightweight) ----

// wasmImports returns the "module.field" of every import in a WASM module, parsing the binary structurally.
// It is defensive: malformed/truncated input yields an error rather than a panic (the promotion gate must
// reject a tampered module, not crash on it).
func wasmImports(b []byte) ([]string, error) {
	if len(b) < 8 || string(b[:4]) != "\x00asm" {
		return nil, fmt.Errorf("not a wasm module")
	}
	p := 8
	var imports []string
	importSections := 0
	for p < len(b) {
		secID := b[p]
		p++
		size, n := uvarint(b, p)
		if n == 0 {
			return nil, fmt.Errorf("bad section size")
		}
		p += n
		end := p + int(size)
		if end > len(b) || end < p {
			return nil, fmt.Errorf("section overruns module")
		}
		if secID == 2 { // import section
			// A well-formed module has AT MOST ONE import section (WASM spec §5.5.2). A second one is
			// malformed AND a smuggling vector (extra imports the gate would otherwise miss if it stopped at
			// the first) — reject it outright rather than scan past it.
			importSections++
			if importSections > 1 {
				return nil, fmt.Errorf("malformed module: multiple import sections")
			}
			imps, err := parseImports(b, p, end)
			if err != nil {
				return nil, err
			}
			imports = imps
		}
		p = end
	}
	return imports, nil
}

func parseImports(b []byte, p, end int) ([]string, error) {
	count, n := uvarint(b, p)
	if n == 0 {
		return nil, fmt.Errorf("bad import count")
	}
	p += n
	var out []string
	for i := uint64(0); i < count; i++ {
		mod, np, err := readName(b, p, end)
		if err != nil {
			return nil, err
		}
		p = np
		fld, np2, err := readName(b, p, end)
		if err != nil {
			return nil, err
		}
		p = np2
		out = append(out, mod+"."+fld)
		if p >= end {
			return nil, fmt.Errorf("truncated import descriptor")
		}
		kind := b[p]
		p++
		switch kind {
		case 0x00: // func: typeidx
			_, n := uvarint(b, p)
			if n == 0 {
				return nil, fmt.Errorf("bad func typeidx")
			}
			p += n
		case 0x01: // table: elemtype + limits
			p++ // elemtype
			np, err := skipLimits(b, p, end)
			if err != nil {
				return nil, err
			}
			p = np
		case 0x02: // mem: limits
			np, err := skipLimits(b, p, end)
			if err != nil {
				return nil, err
			}
			p = np
		case 0x03: // global: valtype + mut
			p += 2
		default:
			return nil, fmt.Errorf("unknown import kind %d", kind)
		}
		if p > end {
			return nil, fmt.Errorf("import descriptor overruns section")
		}
	}
	return out, nil
}

func readName(b []byte, p, end int) (string, int, error) {
	ln, n := uvarint(b, p)
	if n == 0 {
		return "", 0, fmt.Errorf("bad name length")
	}
	p += n
	if p+int(ln) > end {
		return "", 0, fmt.Errorf("name overruns section")
	}
	return string(b[p : p+int(ln)]), p + int(ln), nil
}

func skipLimits(b []byte, p, end int) (int, error) {
	if p >= end {
		return 0, fmt.Errorf("truncated limits")
	}
	flag := b[p]
	p++
	_, n := uvarint(b, p)
	if n == 0 {
		return 0, fmt.Errorf("bad limits min")
	}
	p += n
	if flag == 0x01 {
		_, n := uvarint(b, p)
		if n == 0 {
			return 0, fmt.Errorf("bad limits max")
		}
		p += n
	}
	return p, nil
}

// uvarint decodes a LEB128 unsigned int at b[p:], returning the value and bytes consumed (0 on error).
func uvarint(b []byte, p int) (uint64, int) {
	var x uint64
	var s uint
	for i := 0; p+i < len(b) && i < 10; i++ {
		c := b[p+i]
		if c < 0x80 {
			return x | uint64(c)<<s, i + 1
		}
		x |= uint64(c&0x7f) << s
		s += 7
	}
	return 0, 0
}
