package main

import (
	"fmt"
	"strings"

	wasmcontract "github.com/mnemon-dev/mnemon/harness/core/rule/wasm"
	"github.com/spf13/cobra"
)

var wasmCmd = &cobra.Command{
	Use:    "wasm",
	Short:  "Inspect governed WASM plugins",
	Hidden: true,
}

var wasmInspectCmd = &cobra.Command{
	Use:   "inspect <manifest>",
	Short: "Inspect a governed WASM plugin manifest",
	Args:  cobra.ExactArgs(1),
	RunE:  runWasmInspect,
}

var wasmTestCmd = &cobra.Command{
	Use:   "test <manifest>",
	Short: "Validate a governed WASM plugin manifest",
	Args:  cobra.ExactArgs(1),
	RunE:  runWasmTest,
}

var wasmShadowCmd = &cobra.Command{
	Use:   "shadow <manifest>",
	Short: "Validate a WASM plugin before shadow comparison",
	Args:  cobra.ExactArgs(1),
	RunE:  runWasmShadow,
}

var wasmPromoteCmd = &cobra.Command{
	Use:   "promote <manifest>",
	Short: "Validate a WASM plugin before governed promotion",
	Args:  cobra.ExactArgs(1),
	RunE:  runWasmPromote,
}

func init() {
	wasmCmd.AddCommand(wasmInspectCmd, wasmTestCmd, wasmShadowCmd, wasmPromoteCmd)
	wasmCmd.GroupID = groupAdvanced
	rootCmd.AddCommand(wasmCmd)
}

func runWasmInspect(cmd *cobra.Command, args []string) error {
	inspection, err := inspectWasmManifest(args[0])
	if err != nil {
		return err
	}
	printWasmInspection(cmd, inspection)
	return nil
}

func runWasmTest(cmd *cobra.Command, args []string) error {
	inspection, err := inspectWasmManifest(args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Plugin: %s\n", inspection.Manifest.ID)
	fmt.Fprintln(cmd.OutOrStdout(), "Status: valid")
	return nil
}

func runWasmShadow(cmd *cobra.Command, args []string) error {
	inspection, err := inspectWasmManifest(args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Plugin: %s\n", inspection.Manifest.ID)
	fmt.Fprintln(cmd.OutOrStdout(), "Shadow: ready for governed comparison")
	return nil
}

func runWasmPromote(cmd *cobra.Command, args []string) error {
	inspection, err := inspectWasmManifest(args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Plugin: %s\n", inspection.Manifest.ID)
	fmt.Fprintln(cmd.OutOrStdout(), "Promotion: validation passed; approval required")
	return nil
}

func inspectWasmManifest(path string) (wasmcontract.Inspection, error) {
	manifest, wasmBytes, err := wasmcontract.LoadManifest(path)
	if err != nil {
		return wasmcontract.Inspection{}, err
	}
	return wasmcontract.ValidateManifest(manifest, wasmBytes)
}

func printWasmInspection(cmd *cobra.Command, inspection wasmcontract.Inspection) {
	m := inspection.Manifest
	fmt.Fprintf(cmd.OutOrStdout(), "Plugin: %s\n", m.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "Kind: %s\n", m.Kind)
	fmt.Fprintf(cmd.OutOrStdout(), "Version: %s\n", m.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "Handles: %s\n", strings.Join(m.Handles, ", "))
	fmt.Fprintf(cmd.OutOrStdout(), "Emits: %s\n", strings.Join(m.Emits, ", "))
	fmt.Fprintf(cmd.OutOrStdout(), "Capabilities: %s\n", strings.Join(m.Capabilities, ", "))
	fmt.Fprintf(cmd.OutOrStdout(), "SHA256: %s\n", inspection.SHA256)
	fmt.Fprintln(cmd.OutOrStdout(), "Status: valid")
}
