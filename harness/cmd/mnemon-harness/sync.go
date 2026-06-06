package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/server"
	"github.com/spf13/cobra"
)

var (
	syncRoot            string
	syncStorePath       string
	syncRemotesPath     string
	syncRemoteID        string
	syncRemoteURL       string
	syncRemoteToken     string
	syncRemoteTokenFile string
	syncOnce            bool
	syncBackground      bool
	syncInterval        time.Duration
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync Local Mnemon with Remote Workspace",
}

var syncPushCmd = &cobra.Command{
	Use:   "push --once",
	Short: "Push local accepted changes to Remote Workspace",
	RunE:  runSyncPush,
}

var syncPullCmd = &cobra.Command{
	Use:   "pull --once",
	Short: "Pull Remote Workspace changes into Local Mnemon",
	RunE:  runSyncPull,
}

var syncRunCmd = &cobra.Command{
	Use:   "run --background",
	Short: "Run Remote Workspace sync in the background",
	RunE:  runSyncBackground,
}

func init() {
	syncCmd.PersistentFlags().StringVar(&syncRoot, "root", ".", "project root")
	syncCmd.PersistentFlags().StringVar(&syncStorePath, "store", "", "Local Mnemon store path")
	syncCmd.PersistentFlags().StringVar(&syncRemotesPath, "remotes", "", "Remote Workspace config path")
	syncCmd.PersistentFlags().StringVar(&syncRemoteID, "remote", "default", "Remote Workspace id")
	syncCmd.PersistentFlags().StringVar(&syncRemoteURL, "remote-url", "", "Remote Workspace sync endpoint")
	syncCmd.PersistentFlags().StringVar(&syncRemoteToken, "token", "", "Remote Workspace sync token")
	syncCmd.PersistentFlags().StringVar(&syncRemoteTokenFile, "token-file", "", "Remote Workspace sync token file")
	syncPushCmd.Flags().BoolVar(&syncOnce, "once", false, "push one batch and exit")
	syncPullCmd.Flags().BoolVar(&syncOnce, "once", false, "pull one batch and exit")
	syncRunCmd.Flags().BoolVar(&syncBackground, "background", false, "run until interrupted")
	syncRunCmd.Flags().DurationVar(&syncInterval, "interval", 30*time.Second, "background sync interval")
	syncCmd.AddCommand(syncPushCmd, syncPullCmd, syncRunCmd)
	syncCmd.GroupID = groupSpine
	rootCmd.AddCommand(syncCmd)
}

func runSyncPush(cmd *cobra.Command, args []string) error {
	result, err := syncPushOnce()
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Sync push: %d accepted, %d rejected, %d conflicts\n", result.accepted, result.rejected, result.conflicts)
	return nil
}

func runSyncPull(cmd *cobra.Command, args []string) error {
	result, err := syncPullOnce()
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Sync pull: %d commits\n", result.commits)
	return nil
}

func runSyncBackground(cmd *cobra.Command, args []string) error {
	if !syncBackground {
		return fmt.Errorf("sync run requires --background")
	}
	if syncInterval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		if result, err := syncPushOnce(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "sync push failed: %v\n", err)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Sync push: %d accepted, %d rejected, %d conflicts\n", result.accepted, result.rejected, result.conflicts)
		}
		if result, err := syncPullOnce(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "sync pull failed: %v\n", err)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Sync pull: %d commits\n", result.commits)
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-ticker.C:
		}
	}
}

type syncPushResult struct {
	accepted  int
	rejected  int
	conflicts int
}

type syncPullResult struct {
	commits int
}

func syncPushOnce() (syncPushResult, error) {
	storePath := resolvedSyncStorePath()
	batch, err := server.ReadLocalSyncPushBatch(storePath)
	if err != nil {
		return syncPushResult{}, err
	}
	if len(batch.Commits) == 0 {
		return syncPushResult{}, nil
	}
	remote, err := resolveSyncRemote()
	if err != nil {
		return syncPushResult{}, err
	}
	client := server.NewClientWithToken(remote.Endpoint, remote.Token)
	resp, err := client.SyncPush(server.SyncPushRequest{
		ReplicaID: batch.ReplicaID,
		BatchID:   syncBatchID(batch.ReplicaID, batch.Commits),
		Commits:   batch.Commits,
	})
	if err != nil {
		return syncPushResult{}, fmt.Errorf("sync push failed: %w", err)
	}
	if err := server.ApplyLocalSyncPushResponse(storePath, remote.ID, resp); err != nil {
		return syncPushResult{}, err
	}
	return syncPushResult{accepted: len(resp.Accepted), rejected: len(resp.Rejected), conflicts: len(resp.Conflicts)}, nil
}

func syncPullOnce() (syncPullResult, error) {
	remote, err := resolveSyncRemote()
	if err != nil {
		return syncPullResult{}, err
	}
	storePath := resolvedSyncStorePath()
	state, err := server.ReadLocalSyncPullState(storePath, remote.ID)
	if err != nil {
		return syncPullResult{}, err
	}
	resp, err := server.NewClientWithToken(remote.Endpoint, remote.Token).SyncPull(server.SyncPullRequest{
		ReplicaID:    state.ReplicaID,
		RemoteCursor: state.RemoteCursor,
	})
	if err != nil {
		return syncPullResult{}, fmt.Errorf("sync pull failed: %w", err)
	}
	if err := server.ImportLocalSyncPull(storePath, remote.ID, resp.NextCursor, resp.Commits); err != nil {
		return syncPullResult{}, err
	}
	return syncPullResult{commits: len(resp.Commits)}, nil
}

type syncRemoteConfig struct {
	ID       string
	Endpoint string
	Token    string
}

type syncRemotesDoc struct {
	SchemaVersion int               `json:"schema_version"`
	Remotes       []syncRemoteEntry `json:"remotes"`
}

type syncRemoteEntry struct {
	ID            string `json:"id"`
	Endpoint      string `json:"endpoint"`
	CredentialRef string `json:"credential_ref"`
}

func resolveSyncRemote() (syncRemoteConfig, error) {
	if strings.TrimSpace(syncRemoteURL) != "" {
		tokenFile := syncRemoteTokenFile
		if tokenFile != "" {
			tokenFile = resolveSyncPath(tokenFile)
		}
		token, err := resolveSyncToken(syncRemoteToken, tokenFile)
		if err != nil {
			return syncRemoteConfig{}, err
		}
		return syncRemoteConfig{ID: syncRemoteID, Endpoint: syncRemoteURL, Token: token}, nil
	}
	entry, err := loadSyncRemoteEntry(resolvedSyncRemotesPath(), syncRemoteID)
	if err != nil {
		return syncRemoteConfig{}, err
	}
	token, err := resolveSyncToken(syncRemoteToken, resolveSyncPath(entry.CredentialRef))
	if err != nil {
		return syncRemoteConfig{}, err
	}
	return syncRemoteConfig{ID: entry.ID, Endpoint: entry.Endpoint, Token: token}, nil
}

func loadSyncRemoteEntry(path, id string) (syncRemoteEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return syncRemoteEntry{}, fmt.Errorf("read Remote Workspace config: %w", err)
	}
	var doc syncRemotesDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return syncRemoteEntry{}, fmt.Errorf("parse Remote Workspace config: %w", err)
	}
	if doc.SchemaVersion != 1 {
		return syncRemoteEntry{}, fmt.Errorf("Remote Workspace config schema_version %d unsupported (want 1)", doc.SchemaVersion)
	}
	for _, remote := range doc.Remotes {
		if remote.ID == id {
			if strings.TrimSpace(remote.Endpoint) == "" {
				return syncRemoteEntry{}, fmt.Errorf("Remote Workspace %q has no endpoint", id)
			}
			if strings.TrimSpace(remote.CredentialRef) == "" && strings.TrimSpace(syncRemoteToken) == "" && strings.TrimSpace(syncRemoteTokenFile) == "" {
				return syncRemoteEntry{}, fmt.Errorf("Remote Workspace %q has no credential_ref", id)
			}
			return remote, nil
		}
	}
	return syncRemoteEntry{}, fmt.Errorf("Remote Workspace %q not found in %s", id, path)
}

func resolveSyncToken(token, tokenFile string) (string, error) {
	if strings.TrimSpace(tokenFile) != "" {
		raw, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", fmt.Errorf("read Remote Workspace token file: %w", err)
		}
		token = strings.TrimSpace(string(raw))
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("Remote Workspace sync token is required")
	}
	return token, nil
}

func syncBatchID(replicaID string, commits []contract.LocalCommit) string {
	keys := make([]string, 0, len(commits))
	for _, c := range commits {
		keys = append(keys, strings.Join([]string{
			c.OriginReplicaID,
			c.LocalDecisionID,
			string(c.ResourceRef.Kind),
			string(c.ResourceRef.ID),
			c.FieldsDigest,
		}, "\x00"))
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte(replicaID + "\x00" + strings.Join(keys, "\x00")))
	return "push-" + hex.EncodeToString(sum[:12])
}

func resolvedSyncStorePath() string {
	if syncStorePath != "" {
		return resolveSyncPath(syncStorePath)
	}
	return filepath.Join(syncProjectRoot(), server.DefaultStorePath)
}

func resolvedSyncRemotesPath() string {
	if syncRemotesPath != "" {
		return resolveSyncPath(syncRemotesPath)
	}
	return filepath.Join(syncProjectRoot(), ".mnemon", "harness", "sync", "remotes.json")
}

func resolveSyncPath(path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(syncProjectRoot(), path)
}

func syncProjectRoot() string {
	if syncRoot == "" {
		return "."
	}
	return filepath.Clean(syncRoot)
}
