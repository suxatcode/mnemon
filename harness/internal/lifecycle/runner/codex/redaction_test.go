package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactRefForRedactsFileAndRecordsPreHash(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "prompt-01.txt")
	secret := "token-abcdef123456"
	if err := os.WriteFile(path, []byte("use "+secret+"\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	ref := artifactRefFor(root, "artifact:prompt-01", "command", path, "text/plain")
	if ref.SHA256 == "" {
		t.Fatal("expected redacted artifact sha256")
	}
	if ref.PreRedactionSHA256 == "" {
		t.Fatal("expected pre-redaction sha256")
	}
	if ref.SHA256 == ref.PreRedactionSHA256 {
		t.Fatalf("expected different hashes after redaction, got %s", ref.SHA256)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("secret was not redacted: %s", string(data))
	}
	if !strings.Contains(string(data), "[REDACTED]") {
		t.Fatalf("redaction marker missing: %s", string(data))
	}

	raw := artifactRawObjects([]ArtifactRef{ref})
	if got, _ := raw[0]["pre_redaction_sha256"].(string); got != ref.PreRedactionSHA256 {
		t.Fatalf("raw pre-redaction hash mismatch: %#v", raw[0])
	}
}

func TestArtifactRefForRecordsPreHashForUnchangedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "runner.log")
	if err := os.WriteFile(path, []byte("plain log\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	ref := artifactRefFor(root, "artifact:runner-log", "runner_log", path, "text/plain")
	if ref.SHA256 == "" || ref.PreRedactionSHA256 == "" {
		t.Fatalf("expected both hashes, got %#v", ref)
	}
	if ref.SHA256 != ref.PreRedactionSHA256 {
		t.Fatalf("unchanged file hashes should match, got %#v", ref)
	}
}
