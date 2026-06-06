package main

import (
	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	proposalRoot               string
	proposalID                 string
	proposalRoute              string
	proposalRisk               string
	proposalTitle              string
	proposalSummary            string
	proposalChangeSummary      string
	proposalTargets            []string
	proposalOperations         []string
	proposalEvidence           []string
	proposalValidationSummary  string
	proposalValidationCommands []string
	proposalValidationChecks   []string
	proposalReviewRequired     bool
	proposalReviewScope        string
	proposalRequiredReviews    int
	proposalReviewers          []string
	proposalReviewNotes        string
	proposalScopeStore         string
	proposalScopeHost          string
	proposalScopeLoop          string
	proposalScopeProfileRef    string
	proposalStatus             string
	proposalListStatuses       []string
	proposalSupersededBy       string
	proposalFormat             string
)

var proposalCmd = &cobra.Command{
	Use:   "proposal",
	Short: "Manage Mnemon lifecycle proposals",
	Long:  "Manage project-scoped proposal state under .mnemon/harness/proposals.",
}

var proposalCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a lifecycle proposal draft",
	RunE:  runProposalCreate,
}

var proposalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List lifecycle proposals",
	RunE:  runProposalList,
}

var proposalShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show one lifecycle proposal",
	RunE:  runProposalShow,
}

var proposalUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update proposal fields or transition status",
	RunE:  runProposalUpdate,
}

var proposalApproveCmd = &cobra.Command{
	Use:   "approve",
	Short: "Approve an in-review proposal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProposalTransition(cmd, "approved")
	},
}

var proposalRejectCmd = &cobra.Command{
	Use:   "reject",
	Short: "Reject an in-review or blocked proposal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProposalTransition(cmd, "rejected")
	},
}

var proposalRequestChangesCmd = &cobra.Command{
	Use:   "request-changes",
	Short: "Request changes on an open or in-review proposal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProposalTransition(cmd, "request_changes")
	},
}

var proposalBlockCmd = &cobra.Command{
	Use:   "block",
	Short: "Block an open or in-review proposal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProposalTransition(cmd, "blocked")
	},
}

var proposalApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply an approved proposal",
	RunE:  runProposalApply,
}

var proposalSupersedeCmd = &cobra.Command{
	Use:   "supersede",
	Short: "Mark a proposal superseded",
	RunE:  runProposalSupersede,
}

var proposalWithdrawCmd = &cobra.Command{
	Use:   "withdraw",
	Short: "Withdraw a draft, open, or in-review proposal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProposalTransition(cmd, "withdrawn")
	},
}

var proposalExpireCmd = &cobra.Command{
	Use:   "expire",
	Short: "Expire a stale proposal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProposalTransition(cmd, "expired")
	},
}

func init() {
	proposalCmd.PersistentFlags().StringVar(&proposalRoot, "root", ".", "project root for harness proposal state")

	addProposalContentFlags(proposalCreateCmd, true)
	proposalCreateCmd.Flags().StringVar(&proposalRoute, "route", "memory", "proposal route")
	proposalCreateCmd.Flags().StringVar(&proposalRisk, "risk", "medium", "proposal risk")

	proposalListCmd.Flags().StringArrayVar(&proposalListStatuses, "status", nil, "proposal status; may be repeated")
	proposalListCmd.Flags().StringVar(&proposalFormat, "format", "text", "output format: text or json")

	addProposalIDFlag(proposalShowCmd)
	proposalShowCmd.Flags().StringVar(&proposalFormat, "format", "text", "output format: text or json")

	addProposalIDFlag(proposalUpdateCmd)
	addProposalContentFlags(proposalUpdateCmd, false)
	proposalUpdateCmd.Flags().StringVar(&proposalStatus, "status", "", "target proposal status")
	proposalUpdateCmd.Flags().StringVar(&proposalSupersededBy, "superseded-by", "", "replacement proposal id")

	for _, command := range []*cobra.Command{
		proposalApproveCmd,
		proposalRejectCmd,
		proposalRequestChangesCmd,
		proposalBlockCmd,
		proposalApplyCmd,
		proposalWithdrawCmd,
		proposalExpireCmd,
	} {
		addProposalIDFlag(command)
	}
	addProposalIDFlag(proposalSupersedeCmd)
	proposalSupersedeCmd.Flags().StringVar(&proposalSupersededBy, "superseded-by", "", "replacement proposal id")

	proposalCmd.AddCommand(
		proposalCreateCmd,
		proposalListCmd,
		proposalShowCmd,
		proposalUpdateCmd,
		proposalApproveCmd,
		proposalRejectCmd,
		proposalRequestChangesCmd,
		proposalBlockCmd,
		proposalApplyCmd,
		proposalSupersedeCmd,
		proposalWithdrawCmd,
		proposalExpireCmd,
	)
	proposalCmd.GroupID = groupSpine
	rootCmd.AddCommand(proposalCmd)
}

func addProposalIDFlag(command *cobra.Command) {
	command.Flags().StringVar(&proposalID, "proposal-id", "", "proposal id")
}

func addProposalContentFlags(command *cobra.Command, includeID bool) {
	if includeID {
		addProposalIDFlag(command)
	}
	command.Flags().StringVar(&proposalTitle, "title", "", "proposal title")
	command.Flags().StringVar(&proposalSummary, "summary", "", "proposal summary")
	command.Flags().StringVar(&proposalChangeSummary, "change-summary", "", "change summary")
	command.Flags().StringArrayVar(&proposalTargets, "target", nil, "change target as type=uri; may be repeated")
	command.Flags().StringArrayVar(&proposalOperations, "operation", nil, "operation as type=target=summary; may be repeated")
	command.Flags().StringArrayVar(&proposalEvidence, "evidence", nil, "evidence ref as type=ref or type=ref=summary; may be repeated")
	command.Flags().StringVar(&proposalValidationSummary, "validation-summary", "", "validation plan summary")
	command.Flags().StringArrayVar(&proposalValidationCommands, "validation-command", nil, "validation command; may be repeated")
	command.Flags().StringArrayVar(&proposalValidationChecks, "validation-check", nil, "validation check; may be repeated")
	command.Flags().BoolVar(&proposalReviewRequired, "review-required", false, "require review")
	command.Flags().StringVar(&proposalReviewScope, "review-scope", "", "required review scope")
	command.Flags().IntVar(&proposalRequiredReviews, "required-reviews", 0, "required review count")
	command.Flags().StringArrayVar(&proposalReviewers, "reviewer", nil, "reviewer id; may be repeated")
	command.Flags().StringVar(&proposalReviewNotes, "review-notes", "", "review notes")
	command.Flags().StringVar(&proposalScopeStore, "scope-store", "", "scope memory store")
	command.Flags().StringVar(&proposalScopeHost, "scope-host", "", "scope host id")
	command.Flags().StringVar(&proposalScopeLoop, "scope-loop", "", "scope loop id")
	command.Flags().StringVar(&proposalScopeProfileRef, "scope-profile-ref", "", "scope profile ref")
}

func proposalContentFromFlags() app.ProposalContent {
	return app.ProposalContent{
		Title:              proposalTitle,
		Summary:            proposalSummary,
		ChangeSummary:      proposalChangeSummary,
		Targets:            proposalTargets,
		Operations:         proposalOperations,
		Evidence:           proposalEvidence,
		ValidationSummary:  proposalValidationSummary,
		ValidationCommands: proposalValidationCommands,
		ValidationChecks:   proposalValidationChecks,
		ReviewRequired:     proposalReviewRequired,
		ReviewScope:        proposalReviewScope,
		RequiredReviews:    proposalRequiredReviews,
		Reviewers:          proposalReviewers,
		ReviewNotes:        proposalReviewNotes,
		ScopeStore:         proposalScopeStore,
		ScopeHost:          proposalScopeHost,
		ScopeLoop:          proposalScopeLoop,
		ScopeProfileRef:    proposalScopeProfileRef,
	}
}

func runProposalCreate(cmd *cobra.Command, args []string) error {
	return app.New(proposalRoot).ProposalCreate(cmd.OutOrStdout(), proposalID, proposalRoute, proposalRisk, proposalContentFromFlags())
}

func runProposalList(cmd *cobra.Command, args []string) error {
	return app.New(proposalRoot).ProposalList(cmd.OutOrStdout(), proposalListStatuses, proposalFormat)
}

func runProposalShow(cmd *cobra.Command, args []string) error {
	return app.New(proposalRoot).ProposalShow(cmd.OutOrStdout(), proposalID, proposalFormat)
}

func runProposalUpdate(cmd *cobra.Command, args []string) error {
	return app.New(proposalRoot).ProposalUpdate(cmd.OutOrStdout(), proposalID, proposalStatus, proposalSupersededBy, proposalContentFromFlags())
}

func runProposalApply(cmd *cobra.Command, args []string) error {
	return app.New(proposalRoot).ProposalApply(cmd.OutOrStdout(), proposalID)
}

func runProposalSupersede(cmd *cobra.Command, args []string) error {
	return app.New(proposalRoot).ProposalSupersede(cmd.OutOrStdout(), proposalID, proposalSupersededBy)
}

func runProposalTransition(cmd *cobra.Command, status string) error {
	return app.New(proposalRoot).ProposalTransition(cmd.OutOrStdout(), proposalID, status)
}
