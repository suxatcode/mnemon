package main

import (
	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

const defaultProfileID = "personal-default"

var (
	profileRoot      string
	profileID        string
	profileEntryID   string
	profileEntryType string
	profileSummary   string
	profileContent   string
	profileEvidence  []string
	profileProjectTo []string
	profileHost      string
	profileLoop      string
	profileFormat    string
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage evidence-backed harness profile scope entries",
	Long:  "Manage project-local, evidence-backed profile entries under .mnemon/harness/profiles.",
}

var profileEntryCmd = &cobra.Command{
	Use:   "entry",
	Short: "Manage profile entries",
}

var profileEntryAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Record an evidence-backed profile entry",
	RunE:  runProfileEntryAdd,
}

var profileShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show a profile, optionally filtered by projection target",
	RunE:  runProfileShow,
}

func init() {
	profileCmd.PersistentFlags().StringVar(&profileRoot, "root", ".", "project root for harness profile state")

	profileEntryAddCmd.Flags().StringVar(&profileID, "profile-id", defaultProfileID, "profile id")
	profileEntryAddCmd.Flags().StringVar(&profileEntryID, "entry-id", "", "profile entry id")
	profileEntryAddCmd.Flags().StringVar(&profileEntryType, "type", "", "profile entry type")
	profileEntryAddCmd.Flags().StringVar(&profileSummary, "summary", "", "profile entry summary")
	profileEntryAddCmd.Flags().StringVar(&profileContent, "content", "", "profile entry content")
	profileEntryAddCmd.Flags().StringArrayVar(&profileEvidence, "evidence", nil, "evidence ref as type=ref or type=ref=summary; may be repeated")
	profileEntryAddCmd.Flags().StringArrayVar(&profileProjectTo, "project-to", nil, "projection target as host/loop; may be repeated")

	profileShowCmd.Flags().StringVar(&profileID, "profile-id", defaultProfileID, "profile id")
	profileShowCmd.Flags().StringVar(&profileHost, "host", "", "filter entries projectable to host")
	profileShowCmd.Flags().StringVar(&profileLoop, "loop", "", "filter entries projectable to loop")
	profileShowCmd.Flags().StringVar(&profileFormat, "format", "text", "output format: text or json")

	profileEntryCmd.AddCommand(profileEntryAddCmd)
	profileCmd.AddCommand(profileEntryCmd, profileShowCmd)
	profileCmd.GroupID = groupAdvanced
	rootCmd.AddCommand(profileCmd)
}

func runProfileEntryAdd(cmd *cobra.Command, args []string) error {
	return app.New(profileRoot).ProfileEntryAdd(cmd.OutOrStdout(), app.ProfileEntryInput{
		ProfileID:         profileID,
		EntryID:           profileEntryID,
		Type:              profileEntryType,
		Summary:           profileSummary,
		Content:           profileContent,
		Evidence:          profileEvidence,
		ProjectionTargets: profileProjectTo,
	})
}

func runProfileShow(cmd *cobra.Command, args []string) error {
	return app.New(profileRoot).ProfileShow(cmd.OutOrStdout(), profileID, profileHost, profileLoop, profileFormat)
}
