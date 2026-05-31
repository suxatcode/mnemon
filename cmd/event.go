package cmd

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/internal/daemonemit"
	"github.com/spf13/cobra"
)

var (
	eventRoot          string
	eventPayload       string
	eventCorrelationID string
	eventLoop          string
	eventHost          string
)

var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "Emit Mnemon harness lifecycle events",
}

var eventEmitCmd = &cobra.Command{
	Use:   "emit <topic>",
	Short: "Append one lifecycle event to the harness eventlog",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		payload, err := daemonemit.PayloadFromJSON(eventPayload)
		if err != nil {
			return err
		}
		event, path, err := daemonemit.Emit(daemonemit.Options{
			Root:          eventRoot,
			Topic:         args[0],
			Payload:       payload,
			CorrelationID: eventCorrelationID,
			Loop:          eventLoop,
			Host:          eventHost,
			Actor:         "mnemon-manual",
			Source:        "mnemon.event_emit",
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "emitted %s\n", event.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "path: %s\n", path)
		return nil
	},
}

func init() {
	eventEmitCmd.Flags().StringVar(&eventRoot, "root", ".", "project root whose .mnemon/events.jsonl should receive the event")
	eventEmitCmd.Flags().StringVar(&eventPayload, "payload", "{}", "event payload JSON object")
	eventEmitCmd.Flags().StringVar(&eventCorrelationID, "correlation-id", "", "correlation id; generated when unset")
	eventEmitCmd.Flags().StringVar(&eventLoop, "loop", "", "loop id associated with the event")
	eventEmitCmd.Flags().StringVar(&eventHost, "host", "", "host id associated with the event")
	eventCmd.AddCommand(eventEmitCmd)
	rootCmd.AddCommand(eventCmd)
}
