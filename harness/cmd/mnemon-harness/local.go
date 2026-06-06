package main

import (
	"fmt"
	"io"
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
		storePath := resolvedLocalStorePath()
		fmt.Fprintln(cmd.OutOrStdout(), "Local Mnemon: ready")
		fmt.Fprintln(cmd.OutOrStdout(), "Remote Workspace: disconnected")
		if localBindingsPath != "" {
			bindingsPath := resolvedLocalPath(localBindingsPath)
			loaded, err := server.LoadBindingFile(projectRoot(), bindingsPath)
			if err != nil {
				return err
			}
			return server.RunLocalHTTPServerWithBindings(cmd.Context(), localAddr, storePath, loaded, io.Discard)
		}
		return server.RunHTTPServer(cmd.Context(), localAddr, storePath, io.Discard)
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
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(projectRoot(), path)
}
