package main

import (
	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	auditRoot          string
	auditID            string
	auditKind          string
	auditDecision      string
	auditReason        string
	auditJobID         string
	auditRunnerID      string
	auditProposalRefs  []string
	auditEventRefs     []string
	auditArtifactRefs  []string
	auditSpecJSON      string
	auditEventID       string
	auditLoop          string
	auditHost          string
	auditSource        string
	auditCorrelationID string
	auditCausedBy      string
	auditListKind      string
	auditFormat        string
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Manage Mnemon lifecycle audit records",
	Long:  "Manage project-scoped audit records under .mnemon/harness/audit/records.",
}

var auditAppendCmd = &cobra.Command{
	Use:   "append",
	Short: "Append one lifecycle audit record",
	RunE:  runAuditAppend,
}

var auditListCmd = &cobra.Command{
	Use:   "list",
	Short: "List lifecycle audit records",
	RunE:  runAuditList,
}

var auditShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show one lifecycle audit record",
	RunE:  runAuditShow,
}

var auditVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify audit record and audit event integrity",
	RunE:  runAuditVerify,
}

func init() {
	auditCmd.PersistentFlags().StringVar(&auditRoot, "root", ".", "project root for harness audit state")

	addAuditIDFlag(auditAppendCmd)
	auditAppendCmd.Flags().StringVar(&auditKind, "kind", "manual", "audit kind stored as spec.audit_kind")
	auditAppendCmd.Flags().StringVar(&auditDecision, "decision", "", "audit decision")
	auditAppendCmd.Flags().StringVar(&auditReason, "reason", "", "audit reason")
	auditAppendCmd.Flags().StringVar(&auditJobID, "job-id", "", "job id")
	auditAppendCmd.Flags().StringVar(&auditRunnerID, "runner-id", "", "runner id")
	auditAppendCmd.Flags().StringArrayVar(&auditProposalRefs, "proposal-ref", nil, "proposal ref; may be repeated")
	auditAppendCmd.Flags().StringArrayVar(&auditEventRefs, "event-ref", nil, "event ref; may be repeated")
	auditAppendCmd.Flags().StringArrayVar(&auditArtifactRefs, "artifact-ref", nil, "artifact ref; may be repeated")
	auditAppendCmd.Flags().StringVar(&auditSpecJSON, "spec-json", "", "raw audit spec JSON object")
	auditAppendCmd.Flags().StringVar(&auditEventID, "event-id", "", "audit.recorded event id; generated when unset")
	auditAppendCmd.Flags().StringVar(&auditLoop, "loop", "", "loop id for audit.recorded event")
	auditAppendCmd.Flags().StringVar(&auditHost, "host", "", "host id for audit.recorded event")
	auditAppendCmd.Flags().StringVar(&auditSource, "source", "mnemon.audit", "source for audit.recorded event")
	auditAppendCmd.Flags().StringVar(&auditCorrelationID, "correlation-id", "", "correlation id for audit.recorded event")
	auditAppendCmd.Flags().StringVar(&auditCausedBy, "caused-by", "", "causal event id for audit.recorded event")

	auditListCmd.Flags().StringVar(&auditListKind, "kind", "", "filter by spec.audit_kind")
	auditListCmd.Flags().StringVar(&auditFormat, "format", "text", "output format: text or json")

	addAuditIDFlag(auditShowCmd)
	auditShowCmd.Flags().StringVar(&auditFormat, "format", "text", "output format: text or json")

	auditVerifyCmd.Flags().StringVar(&auditFormat, "format", "text", "output format: text or json")

	auditCmd.AddCommand(auditAppendCmd, auditListCmd, auditShowCmd, auditVerifyCmd)
	rootCmd.AddCommand(auditCmd)
}

func addAuditIDFlag(command *cobra.Command) {
	command.Flags().StringVar(&auditID, "audit-id", "", "audit id")
}

func runAuditAppend(cmd *cobra.Command, args []string) error {
	return app.New(auditRoot).AuditAppend(cmd.OutOrStdout(), app.AuditAppendInput{
		ID:            auditID,
		Kind:          auditKind,
		Decision:      auditDecision,
		Reason:        auditReason,
		JobID:         auditJobID,
		RunnerID:      auditRunnerID,
		ProposalRefs:  auditProposalRefs,
		EventRefs:     auditEventRefs,
		ArtifactRefs:  auditArtifactRefs,
		SpecJSON:      auditSpecJSON,
		EventID:       auditEventID,
		Loop:          auditLoop,
		Host:          auditHost,
		Source:        auditSource,
		CorrelationID: auditCorrelationID,
		CausedBy:      auditCausedBy,
	})
}

func runAuditList(cmd *cobra.Command, args []string) error {
	return app.New(auditRoot).AuditList(cmd.OutOrStdout(), auditListKind, auditFormat)
}

func runAuditShow(cmd *cobra.Command, args []string) error {
	return app.New(auditRoot).AuditShow(cmd.OutOrStdout(), auditID, auditFormat)
}

func runAuditVerify(cmd *cobra.Command, args []string) error {
	return app.New(auditRoot).AuditVerify(cmd.OutOrStdout(), auditFormat)
}
