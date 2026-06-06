package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/server"
)

// SetupOptions configures the `mnemon-harness setup` front door: project a loop into a host runtime
// AND wire the channel (binding entry + optional token + runtime env), so a host agent reaches the
// governed control plane through one channel.
type SetupOptions struct {
	Host        string   // host runtime id, e.g. "codex"
	Loops       []string // loops to project, e.g. ["memory"]
	ControlURL  string   // channel endpoint, e.g. "http://127.0.0.1:8787"
	Principal   string   // authenticated principal, e.g. "codex@project"
	ActorKind   string   // "host-agent" (default) or "control-agent"
	UseToken    bool     // generate + reference a bearer token file (vs trusted-header auth)
	ProjectRoot string   // host projection working dir (defaults to the facade root)
	DryRun      bool     // print all projection + channel changes without writing
}

// SetupResult records the channel artifact paths setup wrote (or would write, on dry-run).
type SetupResult struct {
	BindingFile string
	TokenFile   string
	EnvFile     string
	Changes     []string
}

func channelBase(projectRoot string) string {
	return filepath.Join(projectRoot, ".mnemon", "harness", "channel")
}

func sanitizePrincipal(p string) string {
	return strings.NewReplacer("@", "-", "/", "-", ":", "-").Replace(p)
}

// Setup projects the loops into the host (wrapping the existing declaration-driven loop install — no
// second projector) and writes the channel artifacts. On DryRun it prints every projection + channel
// change without writing.
func (h *Harness) Setup(ctx context.Context, out, errw io.Writer, opts SetupOptions) (SetupResult, error) {
	if opts.Host == "" || opts.Principal == "" || opts.ControlURL == "" {
		return SetupResult{}, fmt.Errorf("setup requires --host, --principal, and --control-url")
	}
	if len(opts.Loops) == 0 {
		return SetupResult{}, fmt.Errorf("setup requires at least one --loop")
	}
	projectRoot := opts.ProjectRoot
	if projectRoot == "" {
		projectRoot = h.root
	}

	// 1. Wrap the existing loop install path (declaration-driven projector). Dry-run lowers to the
	//    projector's own --dry-run so projection changes print without writing.
	action, hostArgs := "install", []string(nil)
	if opts.DryRun {
		hostArgs = []string{"--dry-run"}
	}
	if err := h.LoopProject(ctx, out, errw, action, projectRoot, opts.Host, opts.Loops, hostArgs); err != nil {
		return SetupResult{}, fmt.Errorf("setup: loop install: %w", err)
	}

	// 2. Channel artifacts.
	base := channelBase(projectRoot)
	bindingFile := filepath.Join(base, "bindings.json")
	envFile := filepath.Join(base, "env.sh")
	tokenRel := ""
	tokenFile := ""
	if opts.UseToken {
		tokenRel = filepath.ToSlash(filepath.Join(".mnemon", "harness", "channel", "tokens", sanitizePrincipal(opts.Principal)+".token"))
		tokenFile = filepath.Join(projectRoot, filepath.FromSlash(tokenRel))
	}

	binding := h.channelBinding(opts)
	res := SetupResult{BindingFile: bindingFile, TokenFile: tokenFile, EnvFile: envFile}

	if opts.DryRun {
		res.Changes = append(res.Changes,
			fmt.Sprintf("would upsert channel binding for %s in %s", opts.Principal, bindingFile),
			fmt.Sprintf("would write channel runtime env %s", envFile))
		if opts.UseToken {
			res.Changes = append(res.Changes, fmt.Sprintf("would write bearer token file %s", tokenFile))
		}
		for _, c := range res.Changes {
			fmt.Fprintf(out, "setup(dry-run): %s\n", c)
		}
		return res, nil
	}

	if opts.UseToken {
		if err := writeTokenFile(tokenFile); err != nil {
			return res, err
		}
		res.Changes = append(res.Changes, "wrote bearer token file "+tokenFile)
	}
	if err := server.UpsertBinding(bindingFile, binding, tokenRel); err != nil {
		return res, fmt.Errorf("setup: upsert binding: %w", err)
	}
	res.Changes = append(res.Changes, "upserted channel binding for "+opts.Principal+" in "+bindingFile)
	if err := writeChannelEnv(envFile, opts, tokenRel); err != nil {
		return res, err
	}
	res.Changes = append(res.Changes, "wrote channel runtime env "+envFile)
	for _, c := range res.Changes {
		fmt.Fprintf(out, "setup: %s\n", c)
	}
	return res, nil
}

func (h *Harness) channelBinding(opts SetupOptions) server.ChannelBinding {
	kind := server.KindHostAgent
	if opts.ActorKind == string(server.KindControlAgent) {
		kind = server.KindControlAgent
	}
	observed := []string{"session.observed"}
	var scope []contract.ResourceRef
	for _, loop := range opts.Loops {
		observed = append(observed, loop+".write_candidate_observed")
		scope = append(scope, contract.ResourceRef{Kind: contract.ResourceKind(loop), ID: "project"})
	}
	return server.ChannelBinding{
		Principal:            contract.ActorID(opts.Principal),
		ActorKind:            kind,
		Transport:            server.TransportHTTP,
		Endpoint:             opts.ControlURL,
		AllowedVerbs:         []server.Verb{server.VerbObserve, server.VerbPull, server.VerbStatus},
		AllowedObservedTypes: observed,
		SubscriptionScope:    scope,
		IdempotencyNamespace: "host:" + opts.Principal,
	}
}

func writeTokenFile(path string) error {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("generate token: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(hex.EncodeToString(buf)+"\n"), 0o600)
}

func writeChannelEnv(path string, opts SetupOptions, tokenRel string) error {
	var b strings.Builder
	b.WriteString("# Managed by mnemon-harness setup — channel runtime env (source before host hooks).\n")
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

// SetupStatus reports channel binding health for the project: whether the principal has a binding,
// whether its token file exists, and the recorded control endpoint.
func (h *Harness) SetupStatus(projectRoot, principal string) ([]string, error) {
	if projectRoot == "" {
		projectRoot = h.root
	}
	bindingFile := filepath.Join(channelBase(projectRoot), "bindings.json")
	lines := []string{"channel binding status:", "  binding file: " + bindingFile}
	loaded, err := server.LoadBindingFile(projectRoot, bindingFile)
	if err != nil {
		lines = append(lines, "  bindings: MISSING or invalid ("+err.Error()+")")
		return lines, nil
	}
	found := false
	for _, b := range loaded.Bindings {
		mark := ""
		if principal != "" && string(b.Principal) == principal {
			found = true
			mark = " <-"
		}
		lines = append(lines, fmt.Sprintf("  principal=%s kind=%s endpoint=%s verbs=%d scope=%d%s",
			b.Principal, b.ActorKind, b.Endpoint, len(b.AllowedVerbs), len(b.SubscriptionScope), mark))
	}
	if principal != "" && !found {
		lines = append(lines, "  principal "+principal+": NOT bound")
	}
	return lines, nil
}

// SetupUninstall reverses setup: it uninstalls the loop projections (the existing projector) and
// removes the principal's channel binding + its token file, preserving any other (user-added or
// sibling) binding entries.
func (h *Harness) SetupUninstall(ctx context.Context, out, errw io.Writer, opts SetupOptions) error {
	projectRoot := opts.ProjectRoot
	if projectRoot == "" {
		projectRoot = h.root
	}
	if err := h.LoopProject(ctx, out, errw, "uninstall", projectRoot, opts.Host, opts.Loops, nil); err != nil {
		return fmt.Errorf("setup uninstall: loop uninstall: %w", err)
	}
	base := channelBase(projectRoot)
	if opts.Principal != "" {
		removed, err := server.RemoveBinding(filepath.Join(base, "bindings.json"), contract.ActorID(opts.Principal))
		if err != nil {
			return fmt.Errorf("setup uninstall: remove binding: %w", err)
		}
		if removed {
			fmt.Fprintf(out, "setup uninstall: removed channel binding for %s\n", opts.Principal)
		}
		tokenFile := filepath.Join(base, "tokens", sanitizePrincipal(opts.Principal)+".token")
		if err := os.Remove(tokenFile); err == nil {
			fmt.Fprintf(out, "setup uninstall: removed token file %s\n", tokenFile)
		}
	}
	return nil
}
