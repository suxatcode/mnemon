package hostsurface

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// This file defines the LOOP-SIDE hook intent vocabulary and the HOST-SIDE hook mechanics —
// the two data halves the hook generator (hookgen.go) composes into host hook shells.
//
// Both halves are part of the frozen loop-package/host-mechanics protocol surface:
//   - loops/<loop>/hooks/intents.json   (host-neutral: WHAT a hook does at each timing)
//   - hosts/<host>/host.json "mechanics" (host-specific: HOW stdout/stdin/markers/wording differ)
//
// The vocabulary is a CLOSED set. Unknown timings, gate/section/action kinds, dialects, stdin
// idioms, params, or wording slots are rejected fail-closed: a typo or a future vocabulary
// member must fail loudly instead of silently rendering a hook that drops behavior. Free-form
// prose is confined to the wording slots (and the include fragment file); everything structural
// is enum + parameters so the byte-level shell stays a compile-time template.
//
// Wording convention: intents.json carries the codex wording as the canonical default for every
// slot; hosts that phrase the same slot differently (claude-code) override it via
// mechanics.wording_overrides[loop][timing][slot]. Structural divergence (response dialect,
// stdin idiom, marker presence) is never expressed as wording — it lives in the mechanics enums.

// intentsSchemaVersion pins the intents.json format. A future format must bump this and ship a
// decoder that understands it; an unknown version fails closed.
const intentsSchemaVersion = 1

// hookTimings is the closed set of lifecycle timings a loop can hook (ordering is the canonical
// render/report order, not the JSON map order).
var hookTimings = []string{"prime", "remind", "nudge", "compact"}

// Gate kinds (intent side, per timing).
const (
	gateOncePerSessionMarker = "once-per-session-marker" // prime dedupe: skip if this session already primed
	gateTwoPhaseMarker       = "two-phase-marker"        // compact toggle: first call blocks+marks, second call clears+passes
	gateIfInputField         = "if-input-field"          // exit 0 when the hook input JSON has <field>: true
	gateThreshold            = "threshold"               // compute a count and select wording by comparing to a limit
)

// Threshold metrics and comparators.
const (
	metricFileNonEmptyLines = "file-non-empty-lines" // non-empty lines of <dir>/<file> (memory mirror size)
	metricUsageEventCount   = "usage-event-count"    // non-empty lines of a usage .jsonl (skill evidence count)
	cmpGT                   = "gt"
	cmpGE                   = "ge"
)

// Section kinds (intent side, ordered list; prime-style hooks are section pipelines).
const (
	sectionEnvPrologue     = "env-prologue"      // HOOK_DIR/CONFIG_DIR/ENV_PATH sourcing (+ optional ASSET_DIR/PROJECT_ROOT)
	sectionLocalEnvControl = "local-env-control" // source .mnemon/harness/local/env.sh (+ optional PROJECT_ROOT line)
	sectionControlEnv      = "control-env"       // HARNESS_BIN/CONTROL_ADDR/CONTROL_PRINCIPAL/TOKEN_ARGS
	sectionBanner          = "banner"            // echo lines ("" renders a bare echo)
	sectionControlCall     = "control-call"      // if command -v HARNESS_BIN: observe/status/pull-mirror
	sectionFileEmit        = "file-emit"         // if -f <var>[/<path>]: header + cat
	sectionInclude         = "include"           // splice a loop-side fragment (compile-time concatenation, never evaluated)
)

// Control-call action kinds.
const (
	actionObserve    = "observe"
	actionStatus     = "status"
	actionPullMirror = "pull-mirror"
)

// Response roles (intent side, per timing).
const (
	roleOneLiner = "one-liner" // plain echo, exempt from the host response dialect (remind)
	roleMessage  = "message"   // advisory message; dialect decides plain echo vs JSON envelope
	roleBlock    = "block"     // blocking decision; dialect decides the host's block envelope
)

// Host stdin idioms (mechanics side).
const (
	stdinTolerant   = "tolerant"    // INPUT="$(cat || true)"
	stdinStrict     = "strict"      // INPUT="$(cat)"
	stdinGrepDirect = "grep-direct" // no INPUT capture; the input-field gate pipes cat straight into grep
)

// Host response dialects (mechanics side). Field-name sets and escaping are COMPILE-TIME
// templates; the data only selects among them.
const (
	dialectCodexContinue     = "codex-continue"      // {"continue": false, "stopReason": ..., "systemMessage": ...}
	dialectClaudeDecision    = "claude-decision"     // {"decision": "block", "reason": ...}
	dialectSystemMessageOnly = "system-message-only" // {"systemMessage": ...}
	dialectPlain             = "plain"               // bare echo
)

// Wording slots (the only free-prose surface besides include fragments).
const (
	slotText  = "text"
	slotOver  = "over"
	slotUnder = "under"
)

// HookIntents is the decoded loops/<loop>/hooks/intents.json.
type HookIntents struct {
	SchemaVersion int                     `json:"schema_version"`
	Hooks         map[string]TimingIntent `json:"hooks"`
}

// TimingIntent describes one lifecycle timing: gates that may short-circuit or parameterize the
// hook, an ordered section pipeline (prime), and the response (remind/nudge/compact).
type TimingIntent struct {
	Gates    []HookGate    `json:"gates,omitempty"`
	Sections []HookSection `json:"sections,omitempty"`
	Response *HookResponse `json:"response,omitempty"`
}

// HookGate is the union of all gate kinds; per-kind field checks keep the union closed (a field
// set on the wrong kind is an error, not silently ignored).
type HookGate struct {
	Type string `json:"type"`
	// once-per-session-marker / two-phase-marker
	Marker string `json:"marker,omitempty"`
	// if-input-field
	Field string `json:"field,omitempty"`
	// threshold
	Metric       string `json:"metric,omitempty"`
	Cmp          string `json:"cmp,omitempty"`
	DirEnv       string `json:"dir_env,omitempty"`       // file-non-empty-lines: env var holding the directory
	File         string `json:"file,omitempty"`          // file-non-empty-lines: file name inside the directory
	FileEnv      string `json:"file_env,omitempty"`      // usage-event-count: env var holding the file path
	FileDefault  string `json:"file_default,omitempty"`  // usage-event-count: default file path expression
	LimitEnv     string `json:"limit_env,omitempty"`     // env var holding the limit
	LimitDefault string `json:"limit_default,omitempty"` // default limit value
}

// HookSection is the union of all section kinds (same closed-union discipline as HookGate).
type HookSection struct {
	Type string `json:"type"`
	// Glue attaches this section to the previous one without a blank separator line. Layout is
	// part of the byte-frozen target, and the legacy loops genuinely differ in it, so it is data.
	Glue bool `json:"glue,omitempty"`
	// env-prologue
	AssetDir    bool `json:"asset_dir,omitempty"`
	ProjectRoot bool `json:"project_root,omitempty"`
	// local-env-control
	ProjectRootLine bool `json:"project_root_line,omitempty"`
	// banner
	Lines []string `json:"lines,omitempty"`
	// control-call
	Comment        []string        `json:"comment,omitempty"`
	Actions        []ControlAction `json:"actions,omitempty"`
	WarnMissingBin bool            `json:"warn_missing_bin,omitempty"`
	// file-emit
	Var               string `json:"var,omitempty"`
	Path              string `json:"path,omitempty"`
	Header            string `json:"header,omitempty"`
	BlankBeforeHeader bool   `json:"blank_before_header,omitempty"`
	// include
	Fragment string `json:"fragment,omitempty"`
}

// ControlAction is one channel call inside a control-call section.
type ControlAction struct {
	Type string `json:"type"`
	// observe
	EventType        string `json:"event_type,omitempty"`
	ExternalIDPrefix string `json:"external_id_prefix,omitempty"`
	Payload          string `json:"payload,omitempty"`
	// pull-mirror
	MirrorVar  string `json:"mirror_var,omitempty"`
	MirrorPath string `json:"mirror_path,omitempty"`
}

// HookResponse carries the response role plus its wording slots: either a single text slot or a
// threshold-selected over/under pair (requires a threshold gate).
type HookResponse struct {
	Role  string `json:"role"`
	Text  string `json:"text,omitempty"`
	Over  string `json:"over,omitempty"`
	Under string `json:"under,omitempty"`
}

// HostMechanics is the "mechanics" section of hosts/<host>/host.json: everything host-specific
// the generator needs. define != select — the enums and templates are compiled in; this data
// only selects among them.
type HostMechanics struct {
	StdinRead  MechanicSelection `json:"stdin_read"`
	Dialect    MechanicSelection `json:"dialect"`
	JSONEscape bool              `json:"json_escape"`
	// MarkerOverrides[loop][timing] = false drops a marker gate the intents declare (e.g.
	// claude-code skill prime has no once-per-session marker).
	MarkerOverrides map[string]map[string]bool `json:"marker_overrides,omitempty"`
	// WordingOverrides[loop][timing][slot] replaces the canonical (codex) slot text.
	WordingOverrides map[string]map[string]map[string]string `json:"wording_overrides,omitempty"`
}

// MechanicSelection is a default enum value plus per-(loop,timing) overrides.
type MechanicSelection struct {
	Default   string                       `json:"default"`
	Overrides map[string]map[string]string `json:"overrides,omitempty"`
}

// resolve returns the effective enum value for (loop, timing).
func (s MechanicSelection) resolve(loop, timing string) string {
	if byTiming, ok := s.Overrides[loop]; ok {
		if v, ok := byTiming[timing]; ok {
			return v
		}
	}
	return s.Default
}

// decodeHookIntents is the ONE way intents.json is read: DisallowUnknownFields rejects unknown
// keys at the syntax level (same house rule as capability.decodeSpec), the second Decode requires
// io.EOF so trailing JSON cannot ride along, and validateHookIntents closes the vocabulary at the
// semantic level. Anything this function accepts, the generator can render deterministically.
func decodeHookIntents(raw []byte) (HookIntents, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var intents HookIntents
	if err := dec.Decode(&intents); err != nil {
		return HookIntents{}, err
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return HookIntents{}, fmt.Errorf("trailing data after hook intents (want a single JSON object)")
	}
	if err := validateHookIntents(intents); err != nil {
		return HookIntents{}, err
	}
	return intents, nil
}

// decodeHostMechanics extracts and strictly decodes the "mechanics" section out of a full
// host.json document. The OUTER document is decoded leniently (its other keys belong to the
// manifest validator, not to the hook generator); the mechanics value itself follows the same
// fail-closed decode rule as intents.
func decodeHostMechanics(hostJSON []byte) (HostMechanics, error) {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(hostJSON, &outer); err != nil {
		return HostMechanics{}, fmt.Errorf("parse host.json: %w", err)
	}
	raw, ok := outer["mechanics"]
	if !ok {
		return HostMechanics{}, fmt.Errorf("host.json has no mechanics section (required to generate hooks)")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var mech HostMechanics
	if err := dec.Decode(&mech); err != nil {
		return HostMechanics{}, fmt.Errorf("parse host.json mechanics: %w", err)
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return HostMechanics{}, fmt.Errorf("trailing data after host mechanics (want a single JSON object)")
	}
	if err := validateHostMechanics(mech); err != nil {
		return HostMechanics{}, err
	}
	return mech, nil
}

// --- semantic validation (fail-closed) ---

var (
	envVarNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	shellVarPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	markerNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	fieldNamePattern  = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	relPathPattern    = regexp.MustCompile(`^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`)
	fragmentPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]*\.sh$`)
	numberPattern     = regexp.MustCompile(`^[0-9]+$`)
	// pathExprPattern admits only ${VAR} expansions and path characters: no command
	// substitution, no quotes, no separators — a default path, not a program.
	pathExprPattern = regexp.MustCompile(`^(\$\{[A-Z][A-Z0-9_]*\}|[A-Za-z0-9._/-])+$`)
)

// validateSlotText keeps the free-prose slots inert inside the generated shell: they are
// interpolated inside double quotes (and, for some dialects, inside a JSON heredoc), so quote
// characters, command substitution, and backslashes are rejected rather than escaped — the
// wording surface must not be able to grow into a shell surface.
func validateSlotText(text, where string) error {
	if text == "" {
		return fmt.Errorf("%s: empty text slot", where)
	}
	if strings.ContainsAny(text, "\"`\\\n") || strings.Contains(text, "$(") {
		return fmt.Errorf("%s: text slot contains shell-active characters (quote/backtick/backslash/newline/$()", where)
	}
	return nil
}

func isHookTiming(timing string) bool {
	for _, t := range hookTimings {
		if t == timing {
			return true
		}
	}
	return false
}

// sortedKeys returns map keys in sorted order so validation errors (and any future iteration)
// are deterministic regardless of Go map order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func validateHookIntents(intents HookIntents) error {
	if intents.SchemaVersion != intentsSchemaVersion {
		return fmt.Errorf("unsupported hook intents schema_version %d (want %d)", intents.SchemaVersion, intentsSchemaVersion)
	}
	if len(intents.Hooks) == 0 {
		return fmt.Errorf("hook intents declare no timings")
	}
	for _, timing := range sortedKeys(intents.Hooks) {
		if !isHookTiming(timing) {
			return fmt.Errorf("unknown hook timing %q (closed set: %s)", timing, strings.Join(hookTimings, "|"))
		}
		if err := validateTimingIntent(timing, intents.Hooks[timing]); err != nil {
			return err
		}
	}
	return nil
}

func validateTimingIntent(timing string, intent TimingIntent) error {
	markers, inputFields, thresholds := 0, 0, 0
	for i, gate := range intent.Gates {
		where := fmt.Sprintf("hooks.%s.gates[%d]", timing, i)
		if err := validateGate(gate, where); err != nil {
			return err
		}
		switch gate.Type {
		case gateOncePerSessionMarker, gateTwoPhaseMarker:
			markers++
		case gateIfInputField:
			inputFields++
		case gateThreshold:
			thresholds++
		}
	}
	if markers > 1 || inputFields > 1 || thresholds > 1 {
		return fmt.Errorf("hooks.%s: at most one marker gate, one if-input-field gate, and one threshold gate", timing)
	}
	for i, section := range intent.Sections {
		where := fmt.Sprintf("hooks.%s.sections[%d]", timing, i)
		if err := validateSection(section, where); err != nil {
			return err
		}
	}
	if intent.Response != nil {
		if err := validateResponse(timing, *intent.Response, thresholds > 0); err != nil {
			return err
		}
	}
	return nil
}

func validateGate(gate HookGate, where string) error {
	// forbid fields that do not belong to the declared kind, so a misplaced parameter is an
	// error instead of dead data.
	requireEmpty := func(pairs map[string]string) error {
		for _, key := range sortedKeys(pairs) {
			if pairs[key] != "" {
				return fmt.Errorf("%s: field %q is not valid for gate type %q", where, key, gate.Type)
			}
		}
		return nil
	}
	thresholdFields := map[string]string{
		"metric": gate.Metric, "cmp": gate.Cmp, "dir_env": gate.DirEnv, "file": gate.File,
		"file_env": gate.FileEnv, "file_default": gate.FileDefault,
		"limit_env": gate.LimitEnv, "limit_default": gate.LimitDefault,
	}
	switch gate.Type {
	case gateOncePerSessionMarker, gateTwoPhaseMarker:
		if !markerNamePattern.MatchString(gate.Marker) {
			return fmt.Errorf("%s: invalid marker %q", where, gate.Marker)
		}
		if gate.Field != "" {
			return fmt.Errorf("%s: field %q is not valid for gate type %q", where, "field", gate.Type)
		}
		return requireEmpty(thresholdFields)
	case gateIfInputField:
		if !fieldNamePattern.MatchString(gate.Field) {
			return fmt.Errorf("%s: invalid field %q", where, gate.Field)
		}
		if gate.Marker != "" {
			return fmt.Errorf("%s: field %q is not valid for gate type %q", where, "marker", gate.Type)
		}
		return requireEmpty(thresholdFields)
	case gateThreshold:
		if gate.Marker != "" || gate.Field != "" {
			return fmt.Errorf("%s: marker/field are not valid for gate type %q", where, gate.Type)
		}
		return validateThreshold(gate, where)
	default:
		return fmt.Errorf("%s: unknown gate type %q", where, gate.Type)
	}
}

func validateThreshold(gate HookGate, where string) error {
	if gate.Cmp != cmpGT && gate.Cmp != cmpGE {
		return fmt.Errorf("%s: unknown threshold cmp %q (closed set: %s|%s)", where, gate.Cmp, cmpGT, cmpGE)
	}
	if !envVarNamePattern.MatchString(gate.LimitEnv) {
		return fmt.Errorf("%s: invalid limit_env %q", where, gate.LimitEnv)
	}
	if !numberPattern.MatchString(gate.LimitDefault) {
		return fmt.Errorf("%s: invalid limit_default %q (want a number)", where, gate.LimitDefault)
	}
	switch gate.Metric {
	case metricFileNonEmptyLines:
		if !envVarNamePattern.MatchString(gate.DirEnv) {
			return fmt.Errorf("%s: invalid dir_env %q", where, gate.DirEnv)
		}
		if !relPathPattern.MatchString(gate.File) {
			return fmt.Errorf("%s: invalid file %q", where, gate.File)
		}
		if gate.FileEnv != "" || gate.FileDefault != "" {
			return fmt.Errorf("%s: file_env/file_default are not valid for metric %q", where, gate.Metric)
		}
	case metricUsageEventCount:
		if !envVarNamePattern.MatchString(gate.FileEnv) {
			return fmt.Errorf("%s: invalid file_env %q", where, gate.FileEnv)
		}
		if !pathExprPattern.MatchString(gate.FileDefault) {
			return fmt.Errorf("%s: invalid file_default %q (only ${VAR} expansions and path characters)", where, gate.FileDefault)
		}
		if gate.DirEnv != "" || gate.File != "" {
			return fmt.Errorf("%s: dir_env/file are not valid for metric %q", where, gate.Metric)
		}
	default:
		return fmt.Errorf("%s: unknown threshold metric %q (closed set: %s|%s)", where, gate.Metric, metricFileNonEmptyLines, metricUsageEventCount)
	}
	return nil
}

func validateSection(section HookSection, where string) error {
	type fieldCheck struct {
		name string
		set  bool
	}
	// every field a section instance sets must belong to its declared kind.
	checkOnly := func(allowed ...string) error {
		fields := []fieldCheck{
			{"asset_dir", section.AssetDir},
			{"project_root", section.ProjectRoot},
			{"project_root_line", section.ProjectRootLine},
			{"lines", section.Lines != nil},
			{"comment", section.Comment != nil},
			{"actions", section.Actions != nil},
			{"warn_missing_bin", section.WarnMissingBin},
			{"var", section.Var != ""},
			{"path", section.Path != ""},
			{"header", section.Header != ""},
			{"blank_before_header", section.BlankBeforeHeader},
			{"fragment", section.Fragment != ""},
		}
		allowedSet := map[string]bool{}
		for _, a := range allowed {
			allowedSet[a] = true
		}
		for _, f := range fields {
			if f.set && !allowedSet[f.name] {
				return fmt.Errorf("%s: field %q is not valid for section type %q", where, f.name, section.Type)
			}
		}
		return nil
	}
	switch section.Type {
	case sectionEnvPrologue:
		return checkOnly("asset_dir", "project_root")
	case sectionLocalEnvControl:
		return checkOnly("project_root_line")
	case sectionControlEnv:
		return checkOnly()
	case sectionBanner:
		if err := checkOnly("lines"); err != nil {
			return err
		}
		if len(section.Lines) == 0 {
			return fmt.Errorf("%s: banner requires lines", where)
		}
		for i, line := range section.Lines {
			if line == "" {
				continue // bare echo
			}
			if err := validateSlotText(line, fmt.Sprintf("%s.lines[%d]", where, i)); err != nil {
				return err
			}
		}
		return nil
	case sectionControlCall:
		if err := checkOnly("comment", "actions", "warn_missing_bin"); err != nil {
			return err
		}
		if len(section.Comment) == 0 {
			return fmt.Errorf("%s: control-call requires a comment", where)
		}
		for i, line := range section.Comment {
			if err := validateSlotText(line, fmt.Sprintf("%s.comment[%d]", where, i)); err != nil {
				return err
			}
		}
		if len(section.Actions) == 0 {
			return fmt.Errorf("%s: control-call requires actions", where)
		}
		for i, action := range section.Actions {
			if err := validateControlAction(action, fmt.Sprintf("%s.actions[%d]", where, i)); err != nil {
				return err
			}
		}
		return nil
	case sectionFileEmit:
		if err := checkOnly("var", "path", "header", "blank_before_header"); err != nil {
			return err
		}
		if !shellVarPattern.MatchString(section.Var) {
			return fmt.Errorf("%s: invalid var %q", where, section.Var)
		}
		if section.Path != "" && !relPathPattern.MatchString(section.Path) {
			return fmt.Errorf("%s: invalid path %q", where, section.Path)
		}
		return validateSlotText(section.Header, where+".header")
	case sectionInclude:
		if err := checkOnly("fragment"); err != nil {
			return err
		}
		if !fragmentPattern.MatchString(section.Fragment) {
			return fmt.Errorf("%s: invalid fragment %q", where, section.Fragment)
		}
		return nil
	default:
		return fmt.Errorf("%s: unknown section type %q", where, section.Type)
	}
}

func validateControlAction(action ControlAction, where string) error {
	switch action.Type {
	case actionObserve:
		if !fieldNamePattern.MatchString(strings.ReplaceAll(action.EventType, ".", "_")) {
			return fmt.Errorf("%s: invalid event_type %q", where, action.EventType)
		}
		if !markerNamePattern.MatchString(action.ExternalIDPrefix) {
			return fmt.Errorf("%s: invalid external_id_prefix %q", where, action.ExternalIDPrefix)
		}
		// The payload is spliced inside single quotes in the generated shell: it must be one
		// JSON object and must not be able to break out of the quoting.
		var payload map[string]any
		if err := json.Unmarshal([]byte(action.Payload), &payload); err != nil {
			return fmt.Errorf("%s: payload is not a JSON object: %v", where, err)
		}
		if strings.ContainsAny(action.Payload, "'\n") {
			return fmt.Errorf("%s: payload must not contain single quotes or newlines", where)
		}
		if action.MirrorVar != "" || action.MirrorPath != "" {
			return fmt.Errorf("%s: mirror_var/mirror_path are not valid for action %q", where, action.Type)
		}
		return nil
	case actionStatus:
		if action.EventType != "" || action.ExternalIDPrefix != "" || action.Payload != "" || action.MirrorVar != "" || action.MirrorPath != "" {
			return fmt.Errorf("%s: status takes no parameters", where)
		}
		return nil
	case actionPullMirror:
		if !shellVarPattern.MatchString(action.MirrorVar) {
			return fmt.Errorf("%s: invalid mirror_var %q", where, action.MirrorVar)
		}
		if !relPathPattern.MatchString(action.MirrorPath) {
			return fmt.Errorf("%s: invalid mirror_path %q", where, action.MirrorPath)
		}
		if action.EventType != "" || action.ExternalIDPrefix != "" || action.Payload != "" {
			return fmt.Errorf("%s: observe parameters are not valid for action %q", where, action.Type)
		}
		return nil
	default:
		return fmt.Errorf("%s: unknown control action %q", where, action.Type)
	}
}

func validateResponse(timing string, response HookResponse, hasThreshold bool) error {
	where := fmt.Sprintf("hooks.%s.response", timing)
	switch response.Role {
	case roleOneLiner, roleMessage, roleBlock:
	default:
		return fmt.Errorf("%s: unknown role %q (closed set: %s|%s|%s)", where, response.Role, roleOneLiner, roleMessage, roleBlock)
	}
	hasText := response.Text != ""
	hasSelect := response.Over != "" || response.Under != ""
	switch {
	case hasText && hasSelect:
		return fmt.Errorf("%s: text and over/under are mutually exclusive", where)
	case hasText:
		return validateSlotText(response.Text, where+".text")
	case hasSelect:
		if response.Role == roleOneLiner {
			return fmt.Errorf("%s: role one-liner requires a single text slot", where)
		}
		if response.Over == "" || response.Under == "" {
			return fmt.Errorf("%s: over/under selection requires both slots", where)
		}
		if !hasThreshold {
			return fmt.Errorf("%s: over/under selection requires a threshold gate", where)
		}
		if err := validateSlotText(response.Over, where+".over"); err != nil {
			return err
		}
		return validateSlotText(response.Under, where+".under")
	default:
		return fmt.Errorf("%s: response requires text or over/under slots", where)
	}
}

func validateHostMechanics(mech HostMechanics) error {
	// json_escape=false was the bare-interpolation migration record; no false-using host
	// remains in the tree and the frozen face (host-mechanics-v1) declares the injection face
	// closed PERMANENTLY — so false is now rejected outright (the frozen sentence's enforcement
	// site).
	if !mech.JSONEscape {
		return errors.New("host mechanics: json_escape must be true (bare JSON interpolation is a closed injection face)")
	}
	stdinIdioms := map[string]bool{stdinTolerant: true, stdinStrict: true, stdinGrepDirect: true}
	dialects := map[string]bool{dialectCodexContinue: true, dialectClaudeDecision: true, dialectSystemMessageOnly: true, dialectPlain: true}
	if err := validateMechanicSelection("mechanics.stdin_read", mech.StdinRead, stdinIdioms); err != nil {
		return err
	}
	if err := validateMechanicSelection("mechanics.dialect", mech.Dialect, dialects); err != nil {
		return err
	}
	for _, loop := range sortedKeys(mech.MarkerOverrides) {
		if !markerNamePattern.MatchString(loop) {
			return fmt.Errorf("mechanics.marker_overrides: invalid loop name %q", loop)
		}
		for _, timing := range sortedKeys(mech.MarkerOverrides[loop]) {
			if !isHookTiming(timing) {
				return fmt.Errorf("mechanics.marker_overrides.%s: unknown timing %q", loop, timing)
			}
		}
	}
	for _, loop := range sortedKeys(mech.WordingOverrides) {
		if !markerNamePattern.MatchString(loop) {
			return fmt.Errorf("mechanics.wording_overrides: invalid loop name %q", loop)
		}
		for _, timing := range sortedKeys(mech.WordingOverrides[loop]) {
			if !isHookTiming(timing) {
				return fmt.Errorf("mechanics.wording_overrides.%s: unknown timing %q", loop, timing)
			}
			for _, slot := range sortedKeys(mech.WordingOverrides[loop][timing]) {
				if slot != slotText && slot != slotOver && slot != slotUnder {
					return fmt.Errorf("mechanics.wording_overrides.%s.%s: unknown slot %q (closed set: %s|%s|%s)", loop, timing, slot, slotText, slotOver, slotUnder)
				}
				if err := validateSlotText(mech.WordingOverrides[loop][timing][slot], fmt.Sprintf("mechanics.wording_overrides.%s.%s.%s", loop, timing, slot)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateMechanicSelection(where string, sel MechanicSelection, allowed map[string]bool) error {
	if !allowed[sel.Default] {
		return fmt.Errorf("%s: unknown default %q (closed set: %s)", where, sel.Default, strings.Join(sortedKeys(allowed), "|"))
	}
	for _, loop := range sortedKeys(sel.Overrides) {
		if !markerNamePattern.MatchString(loop) {
			return fmt.Errorf("%s.overrides: invalid loop name %q", where, loop)
		}
		for _, timing := range sortedKeys(sel.Overrides[loop]) {
			if !isHookTiming(timing) {
				return fmt.Errorf("%s.overrides.%s: unknown timing %q", where, loop, timing)
			}
			if value := sel.Overrides[loop][timing]; !allowed[value] {
				return fmt.Errorf("%s.overrides.%s.%s: unknown value %q (closed set: %s)", where, loop, timing, value, strings.Join(sortedKeys(allowed), "|"))
			}
		}
	}
	return nil
}
