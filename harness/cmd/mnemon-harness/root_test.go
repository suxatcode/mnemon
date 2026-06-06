package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRootHelpUsesLocalFirstProductSurface(t *testing.T) {
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		rootCmd.SetArgs(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("root help returned error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"Agent Integration", "Local Mnemon", "Remote Workspace", "memory", "skill", "setup", "local"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected root help to contain %q:\n%s", want, got)
		}
	}
	for _, blocked := range []string{"eval", "goal", "coordination", "runner", "supervisor", "daemon", "proposal"} {
		if strings.Contains(got, blocked) {
			t.Fatalf("root help leaked unsupported product term %q:\n%s", blocked, got)
		}
	}
}
