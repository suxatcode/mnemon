package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	hruntime "github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// codexTeamRuntimeHandle is the in-process Local Mnemon runtime the codex-team-loop demo drives.
// It exists only to host the runtime and satisfy autopilot.Runtime (PullProjection/Submit/
// DecisionLedger live in codex_team_loop.go); the demo's agents are in-process Agents, so there
// is no HTTP control channel here.
type codexTeamRuntimeHandle struct {
	mu sync.RWMutex
	rt *hruntime.Runtime
}

// newCodexTeamRuntimeHandle opens a Local Mnemon runtime over the demo bindings. dynamicRoot and
// tokens are accepted for call-site compatibility but unused: the demo runs fully in-process.
func newCodexTeamRuntimeHandle(storePath, dynamicRoot string, bindings []channel.ChannelBinding, tokens map[string]contract.ActorID) (*codexTeamRuntimeHandle, error) {
	rc, err := app.LocalRuntimeConfigFromBindings(bindings, nil)
	if err != nil {
		return nil, fmt.Errorf("assemble local runtime: %w", err)
	}
	rt, err := hruntime.OpenRuntime(storePath, rc)
	if err != nil {
		return nil, fmt.Errorf("open runtime: %w", err)
	}
	return &codexTeamRuntimeHandle{rt: rt}, nil
}

// Close releases the store and its single-writer lock.
func (h *codexTeamRuntimeHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rt == nil {
		return nil
	}
	err := h.rt.Close()
	h.rt = nil
	return err
}

// codexTeamBindings builds n host-agent bindings (codex-NN@appserver) plus the human@owner
// control-agent, all sharing the wide project-level scope the demo uses. Tokens are minted for
// call-site compatibility; the in-process demo does not authenticate over a channel.
func codexTeamBindings(n int, endpoint string) ([]channel.ChannelBinding, map[string]contract.ActorID, error) {
	refs := []contract.ResourceRef{
		{Kind: "memory", ID: "project"},
		{Kind: "project_intent", ID: "project"},
		{Kind: "assignment", ID: "project"},
		{Kind: "progress_digest", ID: "project"},
		{Kind: "loopdef", ID: "project"},
	}
	observed := []string{
		"session.observed",
		"memory.write_candidate.observed",
		"project_intent.write_candidate.observed",
		"assignment.write_candidate.observed",
		"progress_digest.write_candidate.observed",
		"loopdef.write_candidate.observed",
	}
	bindings := make([]channel.ChannelBinding, 0, n+1)
	tokens := make(map[string]contract.ActorID, n+1)
	for i := 1; i <= n; i++ {
		principal := contract.ActorID(fmt.Sprintf("codex-%02d@appserver", i))
		b := channel.HostAgentBinding(principal, endpoint, refs)
		b.AllowedObservedTypes = observed
		bindings = append(bindings, b)
		tok, err := randomToken()
		if err != nil {
			return nil, nil, err
		}
		tokens[tok] = principal
	}
	operator := channel.ControlAgentBinding("human@owner", endpoint, refs)
	operator.AllowedObservedTypes = observed
	bindings = append(bindings, operator)
	tok, err := randomToken()
	if err != nil {
		return nil, nil, err
	}
	tokens[tok] = "human@owner"
	return bindings, tokens, nil
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func listenerURL(ln net.Listener) string {
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "http://" + ln.Addr().String()
	}
	if host == "" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// codexTeamTrimOutput keeps the last maxRunes runes of s (a bounded tail for prompts/logs).
func codexTeamTrimOutput(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return "... " + string(runes[len(runes)-maxRunes:])
}

// codexTeamOneLine collapses s to its last non-empty line, bounded.
func codexTeamOneLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "no output"
	}
	lines := strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' })
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return codexTeamTrimOutput(line, 240)
		}
	}
	return "no output"
}
