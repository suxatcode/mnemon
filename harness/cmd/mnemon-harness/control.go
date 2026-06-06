package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/server"
	"github.com/spf13/cobra"
)

// The control verbs are the host/control agent's view of the channel (D6): observe pushes an
// observation IN, pull reads the scoped projection OUT, status checks reachability. They reach
// the engine ONLY through server.ServerAPI (the channel client), never kernel/reconcile — the
// same channel a HostAgent and a ControlAgent both speak, differing only by binding/credential.

var (
	controlAddr      string
	controlPrincipal string
	controlToken     string
	controlType      string
	controlPayload   string
	controlExtID     string
	controlActor     string
)

func controlClient() *server.Client {
	if controlToken != "" {
		return server.NewClientWithToken(controlAddr, controlToken)
	}
	return server.NewClient(controlAddr, contract.ActorID(controlPrincipal))
}

var controlCmd = &cobra.Command{
	Use:   "control",
	Short: "Channel client verbs (observe / pull / status) over a running mnemon-harness server",
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
		seq, dup, err := controlClient().Ingest(contract.ActorID(controlPrincipal), contract.ObservationEnvelope{
			ExternalID: controlExtID,
			Event:      contract.Event{Type: controlType, Payload: payload},
		})
		if err != nil {
			return fmt.Errorf("channel observe failed (service unreachable or rejected): %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "observed seq=%d dup=%v\n", seq, dup)
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
		proj, err := controlClient().PullProjection(contract.ActorID(controlPrincipal), contract.Subscription{Actor: contract.ActorID(actor)})
		if err != nil {
			return fmt.Errorf("channel pull failed (service unreachable or unauthorized): %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "projection ref=%s digest=%s resources=%d\n", proj.Ref, proj.Digest, len(proj.Resources))
		return nil
	},
}

var controlStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the channel is reachable and report the principal's projection digest",
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := controlClient().PullProjection(contract.ActorID(controlPrincipal), contract.Subscription{Actor: contract.ActorID(controlPrincipal)})
		if err != nil {
			return fmt.Errorf("channel unreachable or unauthorized: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "channel OK: principal=%s digest=%s\n", controlPrincipal, proj.Digest)
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{controlObserveCmd, controlPullCmd, controlStatusCmd} {
		c.Flags().StringVar(&controlAddr, "addr", "http://127.0.0.1:8787", "server base URL")
		c.Flags().StringVar(&controlPrincipal, "principal", "", "authenticated principal (trusted-header transport)")
		c.Flags().StringVar(&controlToken, "token", "", "bearer token (TokenAuthenticator transport)")
	}
	controlObserveCmd.Flags().StringVar(&controlType, "type", "", "observed event type")
	controlObserveCmd.Flags().StringVar(&controlPayload, "payload", "", "observation payload as JSON")
	controlObserveCmd.Flags().StringVar(&controlExtID, "external-id", "", "idempotency external id")
	controlPullCmd.Flags().StringVar(&controlActor, "actor", "", "subscription actor (defaults to principal)")
	controlCmd.AddCommand(controlObserveCmd, controlPullCmd, controlStatusCmd)
	controlCmd.GroupID = groupSpine
	rootCmd.AddCommand(controlCmd)
}
