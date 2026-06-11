package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/remotesync"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
	"github.com/spf13/cobra"
)

var (
	statusRoot        string
	statusProjectRoot string
	statusPrincipal   string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Agent Integration, Local Mnemon, and Remote Workspace status",
	RunE:  runProductStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusRoot, "root", ".", "repository root containing harness declarations")
	statusCmd.Flags().StringVar(&statusProjectRoot, "project-root", "", "project root for Agent Integration artifacts")
	statusCmd.Flags().StringVar(&statusPrincipal, "principal", "", "Agent Integration principal")
	statusCmd.GroupID = groupSpine
	rootCmd.AddCommand(statusCmd)
}

func runProductStatus(cmd *cobra.Command, args []string) error {
	root := filepath.Clean(statusRoot)
	projectRoot := statusProjectRoot
	if projectRoot == "" {
		projectRoot = root
	}
	projectRoot = filepath.Clean(projectRoot)

	if cfg, err := readLocalConfig(projectRoot); err == nil {
		principal := statusPrincipal
		if principal == "" {
			principal = cfg.Principal
		}
		if st, ok := localServiceStatus(projectRoot, cfg, principal); ok {
			printProductStatus(cmd, true, true, remoteWorkspaceStatus(projectRoot), st.SyncPending, st.SyncSynced, st.SyncConflicts)
			return nil
		}
	}

	lines, err := app.New(root).SetupStatus(projectRoot, statusPrincipal)
	if err != nil {
		return err
	}
	remote := remoteWorkspaceStatus(projectRoot)
	for _, l := range lines {
		if strings.HasPrefix(l, "Remote Workspace:") {
			continue
		}
		fmt.Fprintln(cmd.OutOrStdout(), l)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Remote Workspace: "+remote)
	counts := syncCounts(projectRoot)
	fmt.Fprintf(cmd.OutOrStdout(), "Sync: %d pending, %d synced, %d conflicts\n", counts.Pending, counts.Synced, counts.Conflicts)
	return nil
}

func localServiceStatus(projectRoot string, cfg localConfig, principal string) (contract.ChannelStatus, bool) {
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(principal) == "" {
		return contract.ChannelStatus{}, false
	}
	bindingFile := cfg.BindingFile
	if bindingFile == "" {
		bindingFile = channel.DefaultBindingFile
	}
	loaded, err := channel.LoadBindingFile(projectRoot, resolveProjectPath(projectRoot, bindingFile))
	if err != nil {
		return contract.ChannelStatus{}, false
	}
	client := channel.NewClient(cfg.Endpoint, contract.ActorID(principal))
	if tok := tokenForPrincipal(loaded.Tokens, contract.ActorID(principal)); tok != "" {
		client = channel.NewClientWithToken(cfg.Endpoint, tok)
	}
	st, err := client.Status(contract.ActorID(principal))
	if err != nil {
		return contract.ChannelStatus{}, false
	}
	return st, true
}

func printProductStatus(cmd *cobra.Command, installed, ready bool, remote string, pending, synced, conflicts int) {
	if installed {
		fmt.Fprintln(cmd.OutOrStdout(), "Agent Integration: installed")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Agent Integration: not installed")
	}
	if ready {
		fmt.Fprintln(cmd.OutOrStdout(), "Local Mnemon: ready")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "Local Mnemon: not configured")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Remote Workspace: "+remote)
	fmt.Fprintf(cmd.OutOrStdout(), "Sync: %d pending, %d synced, %d conflicts\n", pending, synced, conflicts)
}

func remoteWorkspaceStatus(projectRoot string) string {
	remote, ok := currentRemoteWorkspace(projectRoot)
	if !ok {
		return "not connected"
	}
	return "connected " + remote
}

func currentRemoteWorkspace(projectRoot string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(projectRoot, ".mnemon", "harness", "sync", "remotes.json"))
	if err != nil {
		return "", false
	}
	var doc remotesync.RemotesDoc
	if err := json.Unmarshal(raw, &doc); err != nil || doc.SchemaVersion != 1 {
		return "", false
	}
	if strings.TrimSpace(doc.Current) != "" {
		return strings.TrimSpace(doc.Current), true
	}
	if len(doc.Remotes) == 1 && strings.TrimSpace(doc.Remotes[0].ID) != "" {
		return strings.TrimSpace(doc.Remotes[0].ID), true
	}
	return "", false
}

func tokenForPrincipal(tokens map[string]contract.ActorID, principal contract.ActorID) string {
	for tok, owner := range tokens {
		if owner == principal {
			return tok
		}
	}
	return ""
}

func syncCounts(projectRoot string) remotesync.LocalSyncCounts {
	storePath := filepath.Join(projectRoot, runtime.DefaultStorePath)
	if _, err := os.Stat(storePath); err != nil {
		return remotesync.LocalSyncCounts{}
	}
	counts, err := remotesync.ReadLocalSyncCounts(storePath)
	if err != nil {
		return remotesync.LocalSyncCounts{}
	}
	return counts
}
