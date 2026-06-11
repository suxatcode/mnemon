package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/spf13/cobra"
)

var (
	localRoot                string
	localAddr                string
	localStorePath           string
	localBindingsPath        string
	localAllowNonLoopback    bool
	localIgnoreExternal      bool
	localAllowInsecureRemote bool
)

var localCmd = &cobra.Command{
	Use:   "local",
	Short: "Run and inspect Local Mnemon",
}

var localRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Local Mnemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		boot, err := resolveLocalBoot()
		if err != nil {
			return err
		}
		addr := localAddr
		if !cmd.Flags().Changed("addr") {
			addr = listenAddrFromEndpoint(boot.Config.Endpoint, localAddr)
		}
		if err := validateListenAddr(addr, localAllowNonLoopback); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Local Mnemon: ready")
		fmt.Fprintln(cmd.OutOrStdout(), "Remote Workspace: disconnected")
		return app.RunLocalHTTPServerWithBindings(cmd.Context(), addr, boot.StorePath, boot.Loaded, app.ServeOptions{
			Loops:               boot.Config.Loops,
			Hosts:               boot.Config.Hosts,
			ProjectRoot:         projectRoot(),
			MirrorMode:          boot.Config.MirrorMode,
			IgnoreExternal:      localIgnoreExternal,
			AllowInsecureRemote: localAllowInsecureRemote,
		}, io.Discard)
	},
}

var localStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Local Mnemon status",
	RunE:  runLocalStatus,
}

var localStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Show how to stop Local Mnemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "Local Mnemon: stop the running local process to shut down")
		return nil
	},
}

func init() {
	localCmd.PersistentFlags().StringVar(&localRoot, "root", ".", "project root")
	localCmd.PersistentFlags().StringVar(&localStorePath, "store", "", "store path; defaults to the project Local Mnemon store")
	localRunCmd.Flags().StringVar(&localAddr, "addr", "127.0.0.1:8787", "listen address")
	localRunCmd.Flags().StringVar(&localBindingsPath, "bindings", "", "Agent Integration binding file")
	localRunCmd.Flags().BoolVar(&localAllowNonLoopback, "allow-nonloopback", false, "explicitly allow listening on a non-loopback address (T1: loopback-only by default)")
	localRunCmd.Flags().BoolVar(&localIgnoreExternal, "ignore-external", false, "boot the embedded-only capability catalog, ignoring external packages under .mnemon/loops (each ignored package is named on stderr)")
	localRunCmd.Flags().BoolVar(&localAllowInsecureRemote, "allow-insecure-remote", false, "let the background sync worker use a plaintext http:// Remote Workspace endpoint with a non-loopback host (T2: fail-closed by default)")
	_ = localRunCmd.Flags().MarkHidden("bindings")
	localCmd.AddCommand(localRunCmd, localStatusCmd, localStopCmd)
	localCmd.GroupID = groupSpine
	rootCmd.AddCommand(localCmd)
}

func runLocalStatus(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(cmd.OutOrStdout(), "Local Mnemon: ready")
	fmt.Fprintf(cmd.OutOrStdout(), "Store: %s\n", resolvedLocalStorePath())
	fmt.Fprintln(cmd.OutOrStdout(), "Mode: local")
	fmt.Fprintln(cmd.OutOrStdout(), "Remote Workspace: disconnected")
	return nil
}

func projectRoot() string {
	if localRoot == "" {
		return "."
	}
	return filepath.Clean(localRoot)
}

func resolvedLocalStorePath() string {
	if localStorePath != "" {
		return resolvedLocalPath(localStorePath)
	}
	return filepath.Join(projectRoot(), runtime.DefaultStorePath)
}

func resolvedLocalPath(path string) string {
	return resolveProjectPath(projectRoot(), path)
}

func resolveProjectPath(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(root, path)
}

// validateListenAddr fail-closes a non-loopback listen address unless explicitly allowed:
// the local control plane is a same-machine governance boundary (T1) — binding 0.0.0.0 or a
// LAN address silently exposes the channel beyond it.
func validateListenAddr(addr string, allowNonLoopback bool) error {
	if allowNonLoopback {
		return nil
	}
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("refusing non-loopback listen address %q (T1 loopback-only); pass --allow-nonloopback to override explicitly", addr)
}

// listenAddrFromEndpoint derives the listen address from the setup-written channel endpoint
// (e.g. "http://127.0.0.1:9001" -> "127.0.0.1:9001"), so a bare `local run` listens where
// setup pointed the hooks/bindings. An empty/unparsable endpoint falls back to fallback.
func listenAddrFromEndpoint(endpoint, fallback string) string {
	if endpoint == "" {
		return fallback
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return fallback
	}
	return u.Host
}

const localNotSetupMessage = "Local Mnemon is not set up.\nRun: mnemon-harness setup --host codex --memory --skills"

var errLocalNotSetup = errors.New(localNotSetupMessage)

type localBoot struct {
	Configured bool
	StorePath  string
	Loaded     channel.LoadedBindings
	Config     localConfig
}

type localConfig struct {
	SchemaVersion int                 `json:"schema_version"`
	Mode          string              `json:"mode"`
	Endpoint      string              `json:"endpoint"`
	Principal     string              `json:"principal"`
	Loops         []string            `json:"loops"`
	Hosts         map[string][]string `json:"hosts"`       // per-host projected loops; absent on old installs (no background re-projection)
	MirrorMode    string              `json:"mirror_mode"` // "manual" | "prime-refresh"; absent defaults to prime-refresh
	BindingFile   string              `json:"binding_file"`
	StorePath     string              `json:"store_path"`
}

func resolveLocalBoot() (localBoot, error) {
	root := projectRoot()
	if localBindingsPath != "" {
		bindingsPath := resolvedLocalPath(localBindingsPath)
		loaded, err := channel.LoadBindingFile(root, bindingsPath)
		if err != nil {
			return localBoot{}, err
		}
		return localBoot{Configured: true, StorePath: resolvedLocalStorePath(), Loaded: loaded}, nil
	}
	cfg, err := readLocalConfig(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return localBoot{}, errLocalNotSetup
		}
		return localBoot{}, err
	}
	bindingPath := cfg.BindingFile
	if bindingPath == "" {
		bindingPath = channel.DefaultBindingFile
	}
	loaded, err := channel.LoadBindingFile(root, resolveProjectPath(root, bindingPath))
	if err != nil {
		return localBoot{}, err
	}
	storePath := resolvedLocalStorePath()
	if localStorePath == "" {
		if cfg.StorePath != "" {
			storePath = resolveProjectPath(root, cfg.StorePath)
		} else {
			storePath = filepath.Join(root, runtime.DefaultStorePath)
		}
	}
	return localBoot{Configured: true, StorePath: storePath, Loaded: loaded, Config: cfg}, nil
}

func readLocalConfig(root string) (localConfig, error) {
	path := filepath.Join(root, ".mnemon", "harness", "local", "config.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return localConfig{}, err
	}
	var cfg localConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return localConfig{}, fmt.Errorf("parse Local Mnemon config: %w", err)
	}
	if cfg.SchemaVersion != 1 {
		return localConfig{}, fmt.Errorf("Local Mnemon config schema_version %d unsupported (want 1)", cfg.SchemaVersion)
	}
	switch cfg.MirrorMode {
	case "":
		cfg.MirrorMode = "prime-refresh"
	case "manual", "prime-refresh":
	default:
		return localConfig{}, fmt.Errorf("Local Mnemon config mirror_mode %q unsupported (manual|prime-refresh)", cfg.MirrorMode)
	}
	return cfg, nil
}
