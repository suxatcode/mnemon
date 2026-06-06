package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/harness/core/server"
	"github.com/spf13/cobra"
)

var (
	localRoot         string
	localAddr         string
	localStorePath    string
	localBindingsPath string
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
		fmt.Fprintln(cmd.OutOrStdout(), "Local Mnemon: ready")
		fmt.Fprintln(cmd.OutOrStdout(), "Remote Workspace: disconnected")
		return server.RunLocalHTTPServerWithBindings(cmd.Context(), localAddr, boot.StorePath, boot.Loaded, io.Discard)
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
	return filepath.Join(projectRoot(), server.DefaultStorePath)
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

const localNotSetupMessage = "Local Mnemon is not set up.\nRun: mnemon-harness setup --host codex --memory --skills"

var errLocalNotSetup = errors.New(localNotSetupMessage)

type localBoot struct {
	Configured bool
	StorePath  string
	Loaded     server.LoadedBindings
	Config     localConfig
}

type localConfig struct {
	SchemaVersion int      `json:"schema_version"`
	Mode          string   `json:"mode"`
	Endpoint      string   `json:"endpoint"`
	Principal     string   `json:"principal"`
	Loops         []string `json:"loops"`
	BindingFile   string   `json:"binding_file"`
	StorePath     string   `json:"store_path"`
}

func resolveLocalBoot() (localBoot, error) {
	root := projectRoot()
	if localBindingsPath != "" {
		bindingsPath := resolvedLocalPath(localBindingsPath)
		loaded, err := server.LoadBindingFile(root, bindingsPath)
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
		bindingPath = server.DefaultBindingFile
	}
	loaded, err := server.LoadBindingFile(root, resolveProjectPath(root, bindingPath))
	if err != nil {
		return localBoot{}, err
	}
	storePath := resolvedLocalStorePath()
	if localStorePath == "" {
		if cfg.StorePath != "" {
			storePath = resolveProjectPath(root, cfg.StorePath)
		} else {
			storePath = filepath.Join(root, server.DefaultStorePath)
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
	return cfg, nil
}
