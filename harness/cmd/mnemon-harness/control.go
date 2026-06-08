package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/hostsurface"
	"github.com/spf13/cobra"
)

// The control verbs are the host/control agent's view of the channel (D6): observe pushes an
// observation IN, pull reads the scoped projection OUT, status checks reachability. They reach
// the engine ONLY through channel.ServerAPI (the channel client), never kernel/reconcile — the
// same channel a HostAgent and a ControlAgent both speak, differing only by binding/credential.

var (
	controlAddr       string
	controlPrincipal  string
	controlToken      string
	controlType       string
	controlPayload    string
	controlExtID      string
	controlActor      string
	controlTokenFile  string
	controlPullJSON   bool
	controlMirrorPath string
	controlStatusJSON bool
)

// controlClient builds the channel client from the resolved credential: a bearer token (from
// --token or, preferring it, --token-file so projected hooks keep the token out of prompt-visible
// command lines), else the trusted principal header.
func controlClient() (*channel.Client, error) {
	token := controlToken
	if controlTokenFile != "" {
		data, err := os.ReadFile(controlTokenFile)
		if err != nil {
			return nil, fmt.Errorf("read --token-file: %w", err)
		}
		token = strings.TrimSpace(string(data))
	}
	if token != "" {
		return channel.NewClientWithToken(controlAddr, token), nil
	}
	return channel.NewClient(controlAddr, contract.ActorID(controlPrincipal)), nil
}

var controlCmd = &cobra.Command{
	Use:    "control",
	Short:  "Channel client verbs (observe / pull / status) over a running Local Mnemon service",
	Hidden: true,
}

var controlObserveCmd = &cobra.Command{
	Use:   "observe",
	Short: "Push an observation into the channel (ServerAPI.Ingest)",
	RunE: func(cmd *cobra.Command, args []string) error {
		var payload map[string]any
		if strings.TrimSpace(controlPayload) != "" {
			if err := json.Unmarshal([]byte(controlPayload), &payload); err != nil {
				return fmt.Errorf("decode --payload: %w", err)
			}
		}
		client, err := controlClient()
		if err != nil {
			return err
		}
		rec, err := client.IngestObserve(contract.ActorID(controlPrincipal), contract.ObservationEnvelope{
			ExternalID: controlExtID,
			Event:      contract.Event{Type: controlType, Payload: payload},
		})
		if err != nil {
			return fmt.Errorf("channel observe failed (service unreachable or rejected): %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "observed seq=%d dup=%v ticked=%v\n", rec.Seq, rec.Dup, rec.Ticked)
		if rec.ProcessingError != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "processing error: %s\n", rec.ProcessingError)
		}
		return nil
	},
}

var controlPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull the principal's scoped projection (ServerAPI.PullProjection)",
	RunE: func(cmd *cobra.Command, args []string) error {
		actor := controlActor
		if actor == "" {
			actor = controlPrincipal
		}
		client, err := controlClient()
		if err != nil {
			return err
		}
		proj, err := client.PullProjection(contract.ActorID(controlPrincipal), contract.Subscription{Actor: contract.ActorID(actor)})
		if err != nil {
			return fmt.Errorf("channel pull failed (service unreachable or unauthorized): %w", err)
		}
		if controlMirrorPath != "" {
			if err := hostsurface.WriteMemoryMirror(controlMirrorPath, proj); err != nil {
				return fmt.Errorf("write memory mirror: %w", err)
			}
			if !controlPullJSON {
				fmt.Fprintf(cmd.OutOrStdout(), "wrote memory mirror %s\n", controlMirrorPath)
			}
		}
		if controlPullJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(proj)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "projection ref=%s digest=%s resources=%d\n", proj.Ref, proj.Digest, len(proj.Resources))
		return nil
	},
}

var controlStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report channel status evidence for the principal (digest, actor kind, store ref, mode)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := controlClient()
		if err != nil {
			return err
		}
		st, err := client.Status(contract.ActorID(controlPrincipal))
		if err != nil {
			return fmt.Errorf("channel unreachable or unauthorized: %w", err)
		}
		if controlStatusJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(st)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Agent Integration: %s\n", st.Principal)
		fmt.Fprintf(cmd.OutOrStdout(), "Local Mnemon: ready (resources=%d, digest=%s)\n", st.Resources, st.Digest)
		fmt.Fprintln(cmd.OutOrStdout(), "Remote Workspace: disconnected")
		fmt.Fprintf(cmd.OutOrStdout(), "Sync: %d pending, %d synced, %d conflicts (local accepted, remote pending)\n", st.SyncPending, st.SyncSynced, st.SyncConflicts)
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{controlObserveCmd, controlPullCmd, controlStatusCmd} {
		c.Flags().StringVar(&controlAddr, "addr", "http://127.0.0.1:8787", "server base URL")
		c.Flags().StringVar(&controlPrincipal, "principal", "", "authenticated principal (trusted-header transport)")
		c.Flags().StringVar(&controlToken, "token", "", "bearer token (TokenAuthenticator transport)")
		c.Flags().StringVar(&controlTokenFile, "token-file", "", "read the bearer token from a file (keeps tokens out of prompt-visible command lines)")
	}
	controlObserveCmd.Flags().StringVar(&controlType, "type", "", "observed event type")
	controlObserveCmd.Flags().StringVar(&controlPayload, "payload", "", "observation payload as JSON")
	controlObserveCmd.Flags().StringVar(&controlExtID, "external-id", "", "idempotency external id")
	controlPullCmd.Flags().StringVar(&controlActor, "actor", "", "subscription actor (defaults to principal)")
	controlPullCmd.Flags().BoolVar(&controlPullJSON, "json", false, "emit scoped projection as JSON")
	controlPullCmd.Flags().StringVar(&controlMirrorPath, "mirror", "", "write MEMORY.md mirror from scoped memory content")
	controlStatusCmd.Flags().BoolVar(&controlStatusJSON, "json", false, "emit channel status as JSON")
	controlCmd.AddCommand(controlObserveCmd, controlPullCmd, controlStatusCmd)
	controlCmd.GroupID = groupSpine
	rootCmd.AddCommand(controlCmd)
}
