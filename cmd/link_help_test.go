package cmd

import (
	"strings"
	"testing"
)

func TestLinkHelpMentionsTeammateMemories(t *testing.T) {
	if !strings.Contains(linkCmd.Short, "teammate") {
		t.Fatalf("link Short should mention teammates, got %q", linkCmd.Short)
	}
	for _, want := range []string{"another teammate's", "shared graph", "Forget"} {
		if !strings.Contains(linkCmd.Long, want) {
			t.Errorf("link Long missing %q", want)
		}
	}
}
