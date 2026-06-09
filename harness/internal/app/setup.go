package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// SetupOptions configures the `mnemon-harness setup` front door: project a loop into a host runtime
// AND wire the channel (binding entry + optional token + runtime env), so a host agent reaches the
// governed control plane through one channel.
type SetupOptions struct {
	Host          string   // host runtime id, e.g. "codex"
	Loops         []string // loops to project, e.g. ["memory"]
	ControlURL    string   // channel endpoint, e.g. "http://127.0.0.1:8787"
	Principal     string   // authenticated principal, e.g. "codex@project"
	ActorKind     string   // "host-agent" (default) or "control-agent"
	UseToken      bool     // generate + reference a bearer token file (vs trusted-header auth)
	TokenExplicit bool     // true when the caller explicitly set UseToken
	ProjectRoot   string   // host projection working dir (defaults to the facade root)
	DryRun        bool     // print all projection + channel changes without writing
}

// SetupResult records the channel artifact paths setup wrote (or would write, on dry-run).
type SetupResult struct {
	BindingFile string
	TokenFile   string
	EnvFile     string
	ConfigFile  string
	Changes     []string
}

func channelBase(projectRoot string) string {
	return filepath.Join(projectRoot, ".mnemon", "harness", "channel")
}

func localBase(projectRoot string) string {
	return filepath.Join(projectRoot, ".mnemon", "harness", "local")
}

func sanitizePrincipal(p string) string {
	return strings.NewReplacer("@", "-", "/", "-", ":", "-").Replace(p)
}

var supportedProductLoops = map[string]bool{
	"memory": true,
	"skill":  true,
}

func validateProductLoops(loops []string) error {
	for _, loop := range loops {
		loop = strings.TrimSpace(loop)
		if loop == "" {
			return fmt.Errorf("setup loop id cannot be empty")
		}
		if !supportedProductLoops[loop] {
			return fmt.Errorf("unsupported product loop %q; setup supports memory and skill", loop)
		}
	}
	return nil
}

// Setup projects the selected loops into the host and writes the Local Mnemon
// channel artifacts. On DryRun it prints every projection + channel change
// without writing.
func (h *Harness) Setup(ctx context.Context, out, errw io.Writer, opts SetupOptions) (SetupResult, error) {
	opts = h.defaultSetupOptions(opts)
	if opts.Host == "" {
		return SetupResult{}, fmt.Errorf("setup requires --host")
	}
	if len(opts.Loops) == 0 {
		return SetupResult{}, fmt.Errorf("setup requires --memory, --skills, or at least one --loop")
	}
	if err := validateProductLoops(opts.Loops); err != nil {
		return SetupResult{}, err
	}
	projectRoot := opts.ProjectRoot

	// 1. Project loop assets. Dry-run lowers to the projector's own --dry-run
	//    so projection changes print without writing.
	action, hostArgs := "install", []string(nil)
	if opts.DryRun {
		hostArgs = []string{"--dry-run"}
	}
	var projectorOut bytes.Buffer
	if err := h.LoopProject(ctx, &projectorOut, errw, action, projectRoot, opts.Host, opts.Loops, hostArgs); err != nil {
		return SetupResult{}, fmt.Errorf("setup: project loop assets: %w", err)
	}

	// 2. Channel artifacts.
	base := channelBase(projectRoot)
	bindingFile := filepath.Join(base, "bindings.json")
	envFile := filepath.Join(localBase(projectRoot), "env.sh")
	configFile := filepath.Join(localBase(projectRoot), "config.json")
	compatEnvFile := filepath.Join(base, "env.sh")
	tokenRel := ""
	tokenFile := ""
	if opts.UseToken {
		tokenRel = filepath.ToSlash(filepath.Join(".mnemon", "harness", "channel", "credentials", sanitizePrincipal(opts.Principal)+".token"))
		tokenFile = filepath.Join(projectRoot, filepath.FromSlash(tokenRel))
	}

	binding := h.channelBinding(opts)
	res := SetupResult{BindingFile: bindingFile, TokenFile: tokenFile, EnvFile: envFile, ConfigFile: configFile}

	if opts.DryRun {
		res.Changes = append(res.Changes,
			fmt.Sprintf("would upsert channel binding for %s in %s", opts.Principal, bindingFile),
			fmt.Sprintf("would write Local Mnemon config %s", configFile),
			fmt.Sprintf("would write Local Mnemon env %s", envFile),
			fmt.Sprintf("would write compatibility env %s", compatEnvFile))
		if opts.UseToken {
			res.Changes = append(res.Changes, fmt.Sprintf("would write bearer token file %s", tokenFile))
		}
		writeSetupSummary(out, opts, true)
		return res, nil
	}

	if opts.UseToken {
		if err := writeTokenFile(tokenFile); err != nil {
			return res, err
		}
		res.Changes = append(res.Changes, "wrote bearer token file "+tokenFile)
	}
	if err := channel.MergeBinding(bindingFile, binding, tokenRel); err != nil {
		return res, fmt.Errorf("setup: merge binding: %w", err)
	}
	res.Changes = append(res.Changes, "upserted channel binding for "+opts.Principal+" in "+bindingFile)
	if err := writeLocalConfig(configFile, opts); err != nil {
		return res, err
	}
	res.Changes = append(res.Changes, "wrote Local Mnemon config "+configFile)
	if err := writeLocalEnv(envFile, opts, tokenRel); err != nil {
		return res, err
	}
	res.Changes = append(res.Changes, "wrote Local Mnemon env "+envFile)
	if err := writeLocalEnv(compatEnvFile, opts, tokenRel); err != nil {
		return res, err
	}
	res.Changes = append(res.Changes, "wrote compatibility env "+compatEnvFile)
	writeSetupSummary(out, opts, false)
	return res, nil
}

func (h *Harness) defaultSetupOptions(opts SetupOptions) SetupOptions {
	opts.Host = strings.TrimSpace(opts.Host)
	if opts.ProjectRoot == "" {
		opts.ProjectRoot = h.root
	}
	if opts.Principal == "" && opts.Host != "" {
		opts.Principal = opts.Host + "@project"
	}
	if opts.ControlURL == "" {
		opts.ControlURL = "http://127.0.0.1:8787"
	}
	if opts.ActorKind == "" {
		opts.ActorKind = string(contract.KindHostAgent)
	}
	if !opts.TokenExplicit {
		opts.UseToken = true
	}
	return opts
}

func writeSetupSummary(out io.Writer, opts SetupOptions, dryRun bool) {
	action := "installed"
	local := "ready"
	if dryRun {
		action = "dry-run install"
		local = "would be ready"
	}
	fmt.Fprintf(out, "Agent Integration: %s for %s (%s)\n", action, displayHost(opts.Host), strings.Join(opts.Loops, ", "))
	fmt.Fprintf(out, "Local Mnemon: %s\n", local)
	fmt.Fprintln(out, "Remote Workspace: not connected")
}

func displayHost(host string) string {
	switch host {
	case "codex":
		return "Codex"
	case "claude-code":
		return "Claude Code"
	default:
		return host
	}
}

func (h *Harness) channelBinding(opts SetupOptions) channel.ChannelBinding {
	kind := contract.KindHostAgent
	if opts.ActorKind == string(contract.KindControlAgent) {
		kind = contract.KindControlAgent
	}
	observed := []string{"session.observed"}
	var scope []contract.ResourceRef
	for _, loop := range opts.Loops {
		// Dual-emit the dotted canonical observed type and its legacy underscore alias, so a host
		// that still sends either form is admitted during the naming convergence (gate-1).
		observed = append(observed, capability.ObservedTypeAndAliases(loop+".write_candidate.observed")...)
		scope = append(scope, contract.ResourceRef{Kind: contract.ResourceKind(loop), ID: "project"})
	}
	return channel.ChannelBinding{
		Principal:            contract.ActorID(opts.Principal),
		ActorKind:            kind,
		Transport:            channel.TransportHTTP,
		Endpoint:             opts.ControlURL,
		AllowedVerbs:         []channel.Verb{channel.VerbObserve, channel.VerbPull, channel.VerbStatus},
		AllowedObservedTypes: observed,
		SubscriptionScope:    scope,
		IdempotencyNamespace: "host:" + opts.Principal,
	}
}

func writeTokenFile(path string) error {
	// Idempotent: keep an existing token so a running Local Mnemon (which holds it in memory) does not
	// get locked out by a rerun rotating it.
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(hex.EncodeToString(buf)+"\n"), 0o600)
}

func writeLocalConfig(path string, opts SetupOptions) error {
	// Union the enabled loops with any already recorded, so installing skill after memory leaves the
	// config naming BOTH loops (additive setup).
	loops := opts.Loops
	if prev, err := os.ReadFile(path); err == nil {
		var existing struct {
			Loops []string `json:"loops"`
		}
		if json.Unmarshal(prev, &existing) == nil {
			loops = unionLoops(existing.Loops, opts.Loops)
		}
	}
	doc := map[string]any{
		"schema_version": 1,
		"mode":           "local",
		"endpoint":       opts.ControlURL,
		"principal":      opts.Principal,
		"loops":          loops,
		"binding_file":   filepath.ToSlash(filepath.Join(".mnemon", "harness", "channel", "bindings.json")),
		"store_path":     filepath.ToSlash(runtime.DefaultStorePath),
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeLocalEnv(path string, opts SetupOptions, tokenRel string) error {
	var b strings.Builder
	b.WriteString("# Managed by mnemon-harness setup - Local Mnemon environment.\n")
	b.WriteString(exportLine("MNEMON_HARNESS_BIN", "mnemon-harness"))
	b.WriteString(exportLine("MNEMON_CONTROL_ADDR", opts.ControlURL))
	b.WriteString(exportLine("MNEMON_CONTROL_PRINCIPAL", opts.Principal))
	if tokenRel != "" {
		b.WriteString(exportLine("MNEMON_CONTROL_TOKEN_FILE", tokenRel))
	}
	for _, loop := range opts.Loops {
		b.WriteString(exportLine("MNEMON_"+strings.ToUpper(loop)+"_LOOP_DIR", filepath.ToSlash(filepath.Join(".mnemon", "harness", loop))))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func exportLine(key, value string) string {
	return fmt.Sprintf("export %s=%q\n", key, value)
}

func unionLoops(a, b []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, ls := range [][]string{a, b} {
		for _, l := range ls {
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	return out
}

// SetupStatus reports the public setup state without exposing local transport
// details. Debug/internal commands can inspect binding files directly.
func (h *Harness) SetupStatus(projectRoot, principal string) ([]string, error) {
	if projectRoot == "" {
		projectRoot = h.root
	}
	bindingFile := filepath.Join(channelBase(projectRoot), "bindings.json")
	loaded, err := channel.LoadBindingFile(projectRoot, bindingFile)
	if err != nil {
		return []string{
			"Agent Integration: not installed",
			"Local Mnemon: not configured",
			"Remote Workspace: not connected",
		}, nil
	}
	found := principal == ""
	for _, b := range loaded.Bindings {
		if principal != "" && string(b.Principal) == principal {
			found = true
			break
		}
	}
	if !found {
		return []string{
			"Agent Integration: installed",
			"Local Mnemon: not configured for this agent",
			"Remote Workspace: not connected",
		}, nil
	}
	return []string{
		"Agent Integration: installed",
		"Local Mnemon: ready",
		"Remote Workspace: not connected",
	}, nil
}

// SetupUninstall reverses setup: it removes projected loop assets and the
// principal's channel binding + token file while preserving sibling bindings.
func (h *Harness) SetupUninstall(ctx context.Context, out, errw io.Writer, opts SetupOptions) error {
	projectRoot := opts.ProjectRoot
	if projectRoot == "" {
		projectRoot = h.root
	}
	if err := h.LoopProject(ctx, out, errw, "uninstall", projectRoot, opts.Host, opts.Loops, nil); err != nil {
		return fmt.Errorf("setup uninstall: remove projected loop assets: %w", err)
	}
	base := channelBase(projectRoot)
	if opts.Principal != "" {
		removed, err := channel.RemoveBinding(filepath.Join(base, "bindings.json"), contract.ActorID(opts.Principal))
		if err != nil {
			return fmt.Errorf("setup uninstall: remove binding: %w", err)
		}
		if removed {
			fmt.Fprintf(out, "setup uninstall: removed channel binding for %s\n", opts.Principal)
		}
		for _, dir := range []string{"credentials", "tokens"} {
			tokenFile := filepath.Join(base, dir, sanitizePrincipal(opts.Principal)+".token")
			if err := os.Remove(tokenFile); err == nil {
				fmt.Fprintf(out, "setup uninstall: removed token file %s\n", tokenFile)
			}
		}
	}
	return nil
}
