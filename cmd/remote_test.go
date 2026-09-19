package cmd

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/suxatcode/mnemon/internal/remoteapi"
)

func TestPrintRemoteResponseWritesWarnings(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	printErr := printRemoteResponse(&remoteapi.Response{
		JSON:     []byte("{\"ok\":true}\n"),
		Warnings: []string{"soft-delete x: boom"},
	})
	_ = w.Close()
	os.Stderr = old
	if printErr != nil {
		t.Fatal(printErr)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "warning: soft-delete x: boom") {
		t.Fatalf("stderr=%q", data)
	}
}
