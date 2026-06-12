package manifest

import (
	"fmt"
	"regexp"
	"strings"
)

// EnvProjectorVars is the CLOSED set of projector-time variables a loop.json env value may
// reference as ${name}; the projector substitutes each to a real path at render time. Anything
// else lowercase in ${...} is rejected (the env injection lock — a value can never name an
// arbitrary projector internal).
var EnvProjectorVars = map[string]bool{"state_dir": true, "host_skills_dir": true}

// envNamePattern namespaces every declared env var so a loop can never overwrite PATH / HOME /
// LD_PRELOAD / BASH_ENV: a name must be MNEMON_-prefixed, uppercase, underscore/digit.
var envNamePattern = regexp.MustCompile(`^MNEMON_[A-Z0-9_]+$`)

// runtimeEnvRefPattern is a bash parameter reference allowed inside a value: ${VAR} or
// ${VAR:-default}, VAR uppercase-namespaced, default restricted to safe literal characters
// (letters, digits, _ . / , - =). No command substitution, no nested expansion.
var runtimeEnvRefPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*(:-[A-Za-z0-9_./,=-]*)?$`)

// envLiteralChar is the safe literal alphabet between ${...} expansions: path and value characters
// only, none of the shell metacharacters ($ ( ) ` " ' \ ; & | < > space newline { }).
func envLiteralChar(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	}
	switch b {
	case '_', '.', '/', ',', '-', '=', ':':
		return true
	}
	return false
}

// validateEnvVar is the env injection lock: a declared env var's NAME must be MNEMON_-namespaced,
// and its VALUE must parse as a sequence of safe literals and ${...} expansions where each
// expansion is either a closed projector variable or a namespaced runtime bash ref. Anything else
// — command substitution, backticks, quotes, backslashes, bare $, unterminated/nested expansions —
// fails closed, so a value spliced into a sourced shell file cannot execute.
func validateEnvVar(name, value string) error {
	if !envNamePattern.MatchString(name) {
		return fmt.Errorf("env name %q must match %s (namespaced; cannot overwrite PATH/HOME/etc.)", name, envNamePattern)
	}
	for i := 0; i < len(value); {
		c := value[i]
		if c == '$' {
			if i+1 >= len(value) || value[i+1] != '{' {
				return fmt.Errorf("env value %q: bare $ (only ${...} expansions allowed)", value)
			}
			end := strings.IndexByte(value[i:], '}')
			if end < 0 {
				return fmt.Errorf("env value %q: unterminated ${ expansion", value)
			}
			inner := value[i+2 : i+end]
			if strings.ContainsAny(inner, "${") {
				return fmt.Errorf("env value %q: nested or malformed expansion %q", value, inner)
			}
			if !EnvProjectorVars[inner] && !runtimeEnvRefPattern.MatchString(inner) {
				return fmt.Errorf("env value %q: expansion ${%s} is neither a closed projector var nor a namespaced runtime ref", value, inner)
			}
			i += end + 1
			continue
		}
		if !envLiteralChar(c) {
			return fmt.Errorf("env value %q: unsafe character %q (shell metacharacters are rejected)", value, string(c))
		}
		i++
	}
	return nil
}

// SubstituteEnvValue resolves the closed projector variables in a (pre-validated) env value; the
// runtime ${VAR:-default} refs pass through verbatim for bash to expand when env.sh is sourced. The
// projector supplies vars keyed by EnvProjectorVars names.
func SubstituteEnvValue(value string, vars map[string]string) string {
	for name, repl := range vars {
		value = strings.ReplaceAll(value, "${"+name+"}", repl)
	}
	return value
}
