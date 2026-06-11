package hostsurface

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
)

// hookgen renders host hook shells from data: loop-side intents (loops/<loop>/hooks/intents.json)
// composed with host-side mechanics (hosts/<host>/host.json "mechanics"). Every shell construct
// is a COMPILE-TIME Go template below; the data only selects vocabulary members and fills wording
// slots. Nothing from the data is evaluated at generation time — include fragments and slot texts
// are concatenated, never executed — so the generated hook is a pure, deterministic function of
// (intents, mechanics): no clock, no environment, no map-iteration order.
//
// The acceptance bar is byte-for-byte parity with the 16 legacy hand-written hook shells
// (hookgen_parity_test.go), which is why the templates preserve the legacy layout exactly —
// including blank lines and comment placement.

// RenderHook renders the hook shell for (loop, host, timing) from the embedded assets.
// It is the generator entry point: pure with respect to everything except assets.FS,
// which is compiled into the binary.
func RenderHook(loop, host, timing string) (string, error) {
	if !markerNamePattern.MatchString(loop) {
		return "", fmt.Errorf("invalid loop name %q", loop)
	}
	if !markerNamePattern.MatchString(host) {
		return "", fmt.Errorf("invalid host name %q", host)
	}
	if !isHookTiming(timing) {
		return "", fmt.Errorf("unknown hook timing %q (closed set: %s)", timing, strings.Join(hookTimings, "|"))
	}
	rawIntents, err := fs.ReadFile(assets.FS, "loops/"+loop+"/hooks/intents.json")
	if err != nil {
		return "", fmt.Errorf("read hook intents for loop %s: %w", loop, err)
	}
	intents, err := decodeHookIntents(rawIntents)
	if err != nil {
		return "", fmt.Errorf("decode hook intents for loop %s: %w", loop, err)
	}
	rawHost, err := fs.ReadFile(assets.FS, "hosts/"+host+"/host.json")
	if err != nil {
		return "", fmt.Errorf("read host.json for host %s: %w", host, err)
	}
	mech, err := decodeHostMechanics(rawHost)
	if err != nil {
		return "", fmt.Errorf("decode host mechanics for host %s: %w", host, err)
	}
	intent, ok := intents.Hooks[timing]
	if !ok {
		return "", fmt.Errorf("loop %s declares no %s hook intent", loop, timing)
	}
	return renderHook(loop, host, timing, intent, mech)
}

// hookRender carries the per-render state: the resolved mechanics for this (loop, timing) plus
// bookkeeping that makes misconfiguration loud (wording overrides that nothing consumed).
type hookRender struct {
	loop, host, timing string
	envPrefix          string // MNEMON_<LOOP>_LOOP
	dirName            string // mnemon-<loop>
	stdin              string // resolved stdin idiom
	dialect            string // resolved response dialect
	escape             bool
	overrides          map[string]string // wording overrides for this (loop, timing)
	consumedSlots      map[string]bool
}

func renderHook(loop, host, timing string, intent TimingIntent, mech HostMechanics) (string, error) {
	r := &hookRender{
		loop:          loop,
		host:          host,
		timing:        timing,
		envPrefix:     "MNEMON_" + strings.ToUpper(strings.ReplaceAll(loop, "-", "_")) + "_LOOP",
		dirName:       "mnemon-" + loop,
		stdin:         mech.StdinRead.resolve(loop, timing),
		dialect:       mech.Dialect.resolve(loop, timing),
		escape:        mech.JSONEscape,
		overrides:     mech.WordingOverrides[loop][timing],
		consumedSlots: map[string]bool{},
	}

	var markerGate, inputGate, thresholdGate *HookGate
	for i := range intent.Gates {
		gate := &intent.Gates[i]
		switch gate.Type {
		case gateOncePerSessionMarker, gateTwoPhaseMarker:
			markerGate = gate
		case gateIfInputField:
			inputGate = gate
		case gateThreshold:
			thresholdGate = gate
		}
	}
	// Host marker override: a host may declare that this (loop, timing) carries no marker
	// (claude-code skill prime). An override that targets a timing without a marker gate is a
	// configuration error, not a no-op.
	if enabled, ok := mech.MarkerOverrides[loop][timing]; ok {
		if markerGate == nil {
			return "", fmt.Errorf("%s/%s/%s: marker override targets a timing with no marker gate", host, loop, timing)
		}
		if !enabled {
			markerGate = nil
		}
	}
	if markerGate != nil && r.stdin == stdinGrepDirect {
		return "", fmt.Errorf("%s/%s/%s: marker gates need a captured INPUT; stdin_read grep-direct is invalid here", host, loop, timing)
	}
	if inputGate != nil && markerGate != nil && r.stdin == stdinGrepDirect {
		return "", fmt.Errorf("%s/%s/%s: grep-direct cannot combine with a marker gate", host, loop, timing)
	}

	var blocks []string
	glue := map[int]bool{} // block index -> attach to previous with "\n" instead of "\n\n"
	add := func(block string) { blocks = append(blocks, block) }
	addGlued := func(block string) {
		glue[len(blocks)] = true
		blocks = append(blocks, block)
	}

	add("#!/usr/bin/env bash\nset -euo pipefail")

	// once-per-session marker renders BEFORE the section pipeline (the legacy prime hooks gate
	// the whole body); the two-phase marker renders after the sections, in the reactive pipeline.
	if markerGate != nil && markerGate.Type == gateOncePerSessionMarker {
		add(r.oncePerSessionMarkerBlock(*markerGate))
	}

	for i, section := range intent.Sections {
		block, err := r.sectionBlock(section)
		if err != nil {
			return "", err
		}
		if section.Glue {
			if i == 0 {
				return "", fmt.Errorf("%s/%s/%s: first section cannot glue to a previous one", host, loop, timing)
			}
			addGlued(block)
		} else {
			add(block)
		}
	}

	// Reactive pipeline: acquire stdin/vars, define json_escape, gate on input fields, compute
	// the threshold, respond. The order is fixed by the vocabulary, not by the data.
	varsLines := thresholdVarsLines(thresholdGate, r.envPrefix)
	if markerGate != nil && markerGate.Type == gateTwoPhaseMarker {
		head, tail := r.twoPhaseMarkerBlocks(*markerGate)
		for _, block := range head {
			add(block)
		}
		add(strings.Join(append([]string{tail}, varsLines...), "\n"))
	} else {
		acquire := []string{}
		if inputGate != nil && r.stdin != stdinGrepDirect {
			acquire = append(acquire, r.inputLine())
		}
		acquire = append(acquire, varsLines...)
		if len(acquire) > 0 {
			add(strings.Join(acquire, "\n"))
		}
	}
	if r.escape && jsonDialect(r.dialect) && intent.Response != nil && intent.Response.Role != roleOneLiner {
		add(jsonEscapeFunction)
	}
	if inputGate != nil {
		add(r.inputGateBlock(*inputGate))
	}
	if thresholdGate != nil {
		add(thresholdComputeBlock(*thresholdGate))
	}
	if intent.Response != nil {
		responseBlocks, err := r.responseBlocks(*intent.Response, thresholdGate)
		if err != nil {
			return "", err
		}
		for _, block := range responseBlocks {
			add(block)
		}
	}

	// Every wording override the host declares for this (loop, timing) must have been consumed:
	// an override that names a slot the intent does not render is a typo, and silently keeping
	// the canonical wording would mask it.
	for _, slot := range sortedKeys(r.overrides) {
		if !r.consumedSlots[slot] {
			return "", fmt.Errorf("%s/%s/%s: wording override for slot %q matches no rendered slot", host, loop, timing, slot)
		}
	}

	var out strings.Builder
	for i, block := range blocks {
		if i > 0 {
			if glue[i] {
				out.WriteString("\n")
			} else {
				out.WriteString("\n\n")
			}
		}
		out.WriteString(block)
	}
	out.WriteString("\n")
	return out.String(), nil
}

// slot resolves a wording slot: host override if present, canonical (codex) text otherwise.
func (r *hookRender) slot(name, canonical string) string {
	r.consumedSlots[name] = true
	if text, ok := r.overrides[name]; ok {
		return text
	}
	return canonical
}

func (r *hookRender) inputLine() string {
	if r.stdin == stdinStrict {
		return `INPUT="$(cat)"`
	}
	return `INPUT="$(cat || true)"`
}

// sessionIDLine extracts session_id from the hook input JSON (host-neutral sed; both hosts send
// the same field).
const sessionIDLine = `SESSION_ID="$(printf '%s' "${INPUT}" | sed -n 's/.*"session_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)"`

func (r *hookRender) oncePerSessionMarkerBlock(gate HookGate) string {
	return strings.Join([]string{
		r.inputLine(),
		sessionIDLine,
		`if [[ -n "${SESSION_ID}" ]]; then`,
		`  MARKER_DIR="${TMPDIR:-/tmp}/` + r.dirName + `"`,
		`  MARKER="${MARKER_DIR}/` + gate.Marker + `-${SESSION_ID}"`,
		`  mkdir -p "${MARKER_DIR}"`,
		`  if [[ -f "${MARKER}" ]]; then`,
		`    exit 0`,
		`  fi`,
		`  touch "${MARKER}"`,
		`fi`,
	}, "\n")
}

// twoPhaseMarkerBlocks returns the leading blocks of the compact toggle plus the "touch" line the
// threshold vars glue onto (the legacy layout keeps touch and the vars in one paragraph).
func (r *hookRender) twoPhaseMarkerBlocks(gate HookGate) ([]string, string) {
	head := []string{
		strings.Join([]string{
			r.inputLine(),
			sessionIDLine,
			`MARKER_DIR="${TMPDIR:-/tmp}/` + r.dirName + `"`,
			`MARKER="${MARKER_DIR}/` + gate.Marker + `-${SESSION_ID:-unknown}"`,
		}, "\n"),
		`mkdir -p "${MARKER_DIR}"`,
		strings.Join([]string{
			`if [[ -f "${MARKER}" ]]; then`,
			`  rm -f "${MARKER}"`,
			`  exit 0`,
			`fi`,
		}, "\n"),
	}
	return head, `touch "${MARKER}"`
}

func (r *hookRender) inputGateBlock(gate HookGate) string {
	pattern := `'"` + gate.Field + `"[[:space:]]*:[[:space:]]*true'`
	source := `printf '%s' "${INPUT}"`
	if r.stdin == stdinGrepDirect {
		source = `cat`
	}
	return strings.Join([]string{
		`if ` + source + ` | grep -q ` + pattern + `; then`,
		`  exit 0`,
		`fi`,
	}, "\n")
}

// jsonEscapeFunction is the compile-time escaper emitted whenever a JSON dialect interpolates a
// shell variable with escaping enabled.
const jsonEscapeFunction = `json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  printf '%s' "${value}"
}`

func jsonDialect(dialect string) bool {
	return dialect == dialectCodexContinue || dialect == dialectClaudeDecision || dialect == dialectSystemMessageOnly
}

// thresholdVarsLines emits the env-var paragraph for a threshold gate. Variable names are part of
// the compiled metric template (the wording slots interpolate them), only env names and defaults
// come from the data.
func thresholdVarsLines(gate *HookGate, envPrefix string) []string {
	if gate == nil {
		return nil
	}
	switch gate.Metric {
	case metricFileNonEmptyLines:
		return []string{
			`MEMORY_DIR="${` + gate.DirEnv + `:-}"`,
			`MEMORY_FILE="${MEMORY_DIR}/` + gate.File + `"`,
			`MAX_NON_EMPTY_LINES="${` + gate.LimitEnv + `:-` + gate.LimitDefault + `}"`,
		}
	case metricUsageEventCount:
		return []string{
			`USAGE_FILE="${` + gate.FileEnv + `:-` + gate.FileDefault + `}"`,
			`REVIEW_MIN_EVENTS="${` + gate.LimitEnv + `:-` + gate.LimitDefault + `}"`,
		}
	}
	return nil
}

func thresholdComputeBlock(gate HookGate) string {
	switch gate.Metric {
	case metricFileNonEmptyLines:
		return strings.Join([]string{
			`if [[ -n "${MEMORY_DIR}" && -f "${MEMORY_FILE}" ]]; then`,
			`  NON_EMPTY_LINES="$(grep -cv '^[[:space:]]*$' "${MEMORY_FILE}" || true)"`,
			`else`,
			`  NON_EMPTY_LINES=0`,
			`fi`,
		}, "\n")
	case metricUsageEventCount:
		return strings.Join([]string{
			`if [[ -f "${USAGE_FILE}" ]]; then`,
			`  EVENT_COUNT="$(grep -cv '^[[:space:]]*$' "${USAGE_FILE}" || true)"`,
			`else`,
			`  EVENT_COUNT=0`,
			`fi`,
		}, "\n")
	}
	return ""
}

// thresholdCondition is the select condition of the over/under wording pair.
func thresholdCondition(gate HookGate) string {
	countVar, limitVar := "NON_EMPTY_LINES", "MAX_NON_EMPTY_LINES"
	if gate.Metric == metricUsageEventCount {
		countVar, limitVar = "EVENT_COUNT", "REVIEW_MIN_EVENTS"
	}
	return `if [[ "${` + countVar + `}" -` + gate.Cmp + ` "${` + limitVar + `}" ]]; then`
}

// responseBlocks renders the response according to the host dialect. Roles map to fixed shell
// variables (message -> MESSAGE, block -> REASON) so the wording slots can interpolate them.
func (r *hookRender) responseBlocks(response HookResponse, thresholdGate *HookGate) ([]string, error) {
	if response.Role == roleOneLiner {
		// dialect-exempt: remind is a plain advisory line on every host.
		return []string{`echo "` + r.slot(slotText, response.Text) + `"`}, nil
	}
	if r.dialect == dialectCodexContinue || r.dialect == dialectClaudeDecision {
		if response.Role != roleBlock {
			return nil, fmt.Errorf("%s/%s/%s: dialect %s requires response role %q", r.host, r.loop, r.timing, r.dialect, roleBlock)
		}
	}
	varName := "MESSAGE"
	if response.Role == roleBlock {
		varName = "REASON"
	}
	selecting := response.Over != "" || response.Under != ""

	if r.dialect == dialectPlain {
		if selecting {
			return []string{strings.Join([]string{
				thresholdCondition(*thresholdGate),
				`  echo "` + r.slot(slotOver, response.Over) + `"`,
				`else`,
				`  echo "` + r.slot(slotUnder, response.Under) + `"`,
				`fi`,
			}, "\n")}, nil
		}
		return []string{`echo "` + r.slot(slotText, response.Text) + `"`}, nil
	}

	var assign string
	if selecting {
		assign = strings.Join([]string{
			thresholdCondition(*thresholdGate),
			`  ` + varName + `="` + r.slot(slotOver, response.Over) + `"`,
			`else`,
			`  ` + varName + `="` + r.slot(slotUnder, response.Under) + `"`,
			`fi`,
		}, "\n")
	} else {
		assign = varName + `="` + r.slot(slotText, response.Text) + `"`
	}

	value := `${` + varName + `}`
	if r.escape {
		value = `$(json_escape "${` + varName + `}")`
	}
	var body []string
	switch r.dialect {
	case dialectCodexContinue:
		body = []string{
			`  "continue": false,`,
			`  "stopReason": "` + value + `",`,
			`  "systemMessage": "` + value + `"`,
		}
	case dialectClaudeDecision:
		body = []string{
			`  "decision": "block",`,
			`  "reason": "` + value + `"`,
		}
	case dialectSystemMessageOnly:
		body = []string{
			`  "systemMessage": "` + value + `"`,
		}
	default:
		return nil, fmt.Errorf("%s/%s/%s: unknown response dialect %q", r.host, r.loop, r.timing, r.dialect)
	}
	heredoc := strings.Join(append(append([]string{`cat <<JSON`, `{`}, body...), `}`, `JSON`), "\n")
	return []string{assign, heredoc}, nil
}

// --- section templates ---

func (r *hookRender) sectionBlock(section HookSection) (string, error) {
	switch section.Type {
	case sectionEnvPrologue:
		lines := []string{
			`HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"`,
			`CONFIG_DIR="$(cd "${HOOK_DIR}/../.." && pwd)"`,
			`ENV_PATH="${` + r.envPrefix + `_ENV:-${CONFIG_DIR}/` + r.dirName + `/env.sh}"`,
			`if [[ -f "${ENV_PATH}" ]]; then`,
			`  # shellcheck source=/dev/null`,
			`  source "${ENV_PATH}"`,
			`fi`,
		}
		if section.AssetDir {
			lines = append(lines, `ASSET_DIR="${`+r.envPrefix+`_DIR:-${CONFIG_DIR}/`+r.dirName+`}"`)
		}
		if section.ProjectRoot {
			lines = append(lines, `PROJECT_ROOT="$(cd "${CONFIG_DIR}/.." && pwd)"`)
		}
		return strings.Join(lines, "\n"), nil
	case sectionLocalEnvControl:
		lines := []string{}
		if section.ProjectRootLine {
			lines = append(lines, `PROJECT_ROOT="$(cd "${CONFIG_DIR}/.." && pwd)"`)
		}
		lines = append(lines,
			"# Local Mnemon env (MNEMON_HARNESS_BIN / MNEMON_CONTROL_*), written by `mnemon-harness setup`.",
			`LOCAL_ENV="${PROJECT_ROOT}/.mnemon/harness/local/env.sh"`,
			`if [[ -f "${LOCAL_ENV}" ]]; then`,
			`  # shellcheck source=/dev/null`,
			`  source "${LOCAL_ENV}"`,
			`fi`,
		)
		return strings.Join(lines, "\n"), nil
	case sectionControlEnv:
		return strings.Join([]string{
			`HARNESS_BIN="${MNEMON_HARNESS_BIN:-mnemon-harness}"`,
			`CONTROL_ADDR="${MNEMON_CONTROL_ADDR:-http://127.0.0.1:8787}"`,
			`CONTROL_PRINCIPAL="${MNEMON_CONTROL_PRINCIPAL:-}"`,
			`TOKEN_ARGS=()`,
			`if [[ -n "${MNEMON_CONTROL_TOKEN_FILE:-}" ]]; then`,
			`  TOKEN_PATH="${MNEMON_CONTROL_TOKEN_FILE}"`,
			`  if [[ "${TOKEN_PATH}" != /* ]]; then`,
			`    TOKEN_PATH="${PROJECT_ROOT}/${TOKEN_PATH}"`,
			`  fi`,
			`  TOKEN_ARGS=(--token-file "${TOKEN_PATH}")`,
			`fi`,
		}, "\n"), nil
	case sectionBanner:
		lines := make([]string, 0, len(section.Lines))
		for _, line := range section.Lines {
			if line == "" {
				lines = append(lines, `echo`)
			} else {
				lines = append(lines, `echo "`+line+`"`)
			}
		}
		return strings.Join(lines, "\n"), nil
	case sectionControlCall:
		return r.controlCallBlock(section)
	case sectionFileEmit:
		target := `${` + section.Var + `}`
		if section.Path != "" {
			target += `/` + section.Path
		}
		lines := []string{`if [[ -f "` + target + `" ]]; then`}
		if section.BlankBeforeHeader {
			lines = append(lines, `  echo`)
		}
		lines = append(lines,
			`  echo "`+section.Header+`"`,
			`  cat "`+target+`"`,
			`fi`,
		)
		return strings.Join(lines, "\n"), nil
	case sectionInclude:
		return r.includeBlock(section.Fragment)
	}
	return "", fmt.Errorf("%s/%s/%s: unknown section type %q", r.host, r.loop, r.timing, section.Type)
}

func (r *hookRender) controlCallBlock(section HookSection) (string, error) {
	lines := []string{}
	for _, comment := range section.Comment {
		lines = append(lines, `# `+comment)
	}
	lines = append(lines, `if command -v "${HARNESS_BIN}" >/dev/null 2>&1; then`)
	for _, action := range section.Actions {
		switch action.Type {
		case actionObserve:
			lines = append(lines,
				`  "${HARNESS_BIN}" control observe \`,
				`    --type `+action.EventType+` \`,
				`    --addr "${CONTROL_ADDR}" \`,
				`    --principal "${CONTROL_PRINCIPAL}" \`,
				`    ${TOKEN_ARGS[@]+"${TOKEN_ARGS[@]}"} \`,
				`    --external-id "`+action.ExternalIDPrefix+`-${SESSION_ID:-session}" \`,
				`    --payload '`+action.Payload+`' \`,
				`    >/dev/null 2>&1 || true`,
			)
		case actionStatus:
			lines = append(lines,
				`  "${HARNESS_BIN}" control status \`,
				`    --addr "${CONTROL_ADDR}" \`,
				`    --principal "${CONTROL_PRINCIPAL}" \`,
				`    ${TOKEN_ARGS[@]+"${TOKEN_ARGS[@]}"} 2>/dev/null || echo "Warning: Local Mnemon status unavailable."`,
			)
		case actionPullMirror:
			lines = append(lines,
				`  if [[ -n "${CONTROL_PRINCIPAL}" ]]; then`,
				`    "${HARNESS_BIN}" control pull \`,
				`      --addr "${CONTROL_ADDR}" \`,
				`      --principal "${CONTROL_PRINCIPAL}" \`,
				`      ${TOKEN_ARGS[@]+"${TOKEN_ARGS[@]}"} \`,
				`      --mirror "${`+action.MirrorVar+`}/`+action.MirrorPath+`" \`,
				`      >/dev/null 2>&1 || true`,
				`  fi`,
			)
		default:
			return "", fmt.Errorf("%s/%s/%s: unknown control action %q", r.host, r.loop, r.timing, action.Type)
		}
	}
	if section.WarnMissingBin {
		lines = append(lines,
			`else`,
			`  echo "Warning: ${HARNESS_BIN} binary is not available in PATH."`,
		)
	}
	lines = append(lines, `fi`)
	return strings.Join(lines, "\n"), nil
}

// includeBlock splices a loop-side fragment verbatim. Fragments are loop-package DATA shipped
// next to intents.json; they are concatenated at generation time and never evaluated by the
// generator, and the embedded-only sourcing keeps them inside the trusted asset set.
func (r *hookRender) includeBlock(fragment string) (string, error) {
	data, err := fs.ReadFile(assets.FS, "loops/"+r.loop+"/hooks/fragments/"+fragment)
	if err != nil {
		return "", fmt.Errorf("%s/%s/%s: read fragment %s: %w", r.host, r.loop, r.timing, fragment, err)
	}
	content := strings.TrimSuffix(string(data), "\n")
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("%s/%s/%s: fragment %s is empty", r.host, r.loop, r.timing, fragment)
	}
	return content, nil
}

// DeclaredHookTimings returns the lifecycle timings the loop's intents declare, in canonical
// order. A loop without an intents file is a hook-less loop (nil, nil) — legitimate, not an
// error; a present-but-invalid intents file fails closed.
func DeclaredHookTimings(loop string) ([]string, error) {
	if !markerNamePattern.MatchString(loop) {
		return nil, fmt.Errorf("invalid loop name %q", loop)
	}
	raw, err := fs.ReadFile(assets.FS, "loops/"+loop+"/hooks/intents.json")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	intents, err := decodeHookIntents(raw)
	if err != nil {
		return nil, fmt.Errorf("decode hook intents for loop %s: %w", loop, err)
	}
	var out []string
	for _, t := range hookTimings {
		if _, ok := intents.Hooks[t]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// ValidateGeneratedHooks is the loop-validate-time gate for the generated hook surface: every
// (host, loop) pair must render ALL its declared timings cleanly. This catches what schema
// validation alone cannot — fragment files missing, host mechanics that no template combination
// supports, wording overrides nothing consumes — BEFORE an install would fail closed at
// projection time.
func ValidateGeneratedHooks(hosts, loops []string) ([]string, error) {
	var lines []string
	for _, loop := range loops {
		timings, err := DeclaredHookTimings(loop)
		if err != nil {
			return nil, fmt.Errorf("loop %s: %w", loop, err)
		}
		if len(timings) == 0 {
			continue
		}
		for _, host := range hosts {
			for _, timing := range timings {
				if _, err := RenderHook(loop, host, timing); err != nil {
					return nil, fmt.Errorf("render %s/%s/%s: %w", host, loop, timing, err)
				}
			}
			lines = append(lines, fmt.Sprintf("hooks %s/%s: %d generated timings OK", host, loop, len(timings)))
		}
	}
	return lines, nil
}
