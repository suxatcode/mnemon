package main

import (
	"github.com/mnemon-dev/mnemon/harness/core/server"
	"github.com/spf13/cobra"
)

var (
	serverAddr      string
	serverStorePath string
)

// serverCmd + demoCmd fold the former standalone mnemon-control binary into the one harness
// binary (D2). Both reach the engine only through the channel package (server.ServerAPI /
// server.RunDemo), never kernel/reconcile directly (the P2.3 boundary, enforced by ringguard).

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the core control-plane channel (observe/pull) over httpapi",
	Long:  "Boot a ControlServer over a persistent kernel store and serve the channel (ServerAPI: observe via Ingest, pull via PullProjection) over httpapi until interrupted.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.RunHTTPServer(cmd.Context(), serverAddr, serverStorePath, cmd.OutOrStdout())
	},
}

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Run the self-checking full control-plane demo (exits 0 iff every link holds)",
	Long:  "Boot a ControlServer whose rule seat holds a real wazero WASM rule and drive two edges through the whole governed chain (deny/propose, CAS, conflict, scoped projection, job lane, receipt, tampered-readback, masked replay). Exits 0 iff every link holds.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return server.RunDemo(cmd.OutOrStdout())
	},
}

func init() {
	serverCmd.Flags().StringVar(&serverAddr, "addr", "127.0.0.1:8787", "listen address")
	serverCmd.Flags().StringVar(&serverStorePath, "store", server.DefaultStorePath, "kernel store path")
	serverCmd.GroupID = groupSpine
	demoCmd.GroupID = groupAdvanced
	rootCmd.AddCommand(serverCmd, demoCmd)
}
