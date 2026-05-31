package codex

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
)

type Redactor interface {
	Redact([]byte) ([]byte, bool, error)
}

type RegexRedactor struct {
	Patterns    []*regexp.Regexp
	Replacement []byte
}

func DefaultArtifactRedactor() RegexRedactor {
	return RegexRedactor{
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(sk-|api-|token-|bearer\s+)[a-zA-Z0-9_-]{8,}`),
		},
		Replacement: []byte("[REDACTED]"),
	}
}

func (r RegexRedactor) Redact(data []byte) ([]byte, bool, error) {
	if len(r.Patterns) == 0 {
		return append([]byte(nil), data...), false, nil
	}
	replacement := r.Replacement
	if replacement == nil {
		replacement = []byte("[REDACTED]")
	}
	out := append([]byte(nil), data...)
	changed := false
	for _, pattern := range r.Patterns {
		if pattern == nil {
			continue
		}
		next := pattern.ReplaceAll(out, replacement)
		if string(next) != string(out) {
			changed = true
		}
		out = next
	}
	return out, changed, nil
}

func redactArtifactFile(path string, redactor Redactor) (string, error) {
	if redactor == nil {
		return "", nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read artifact for redaction: %w", err)
	}
	preHash := "sha256:" + sha256Hex(data)
	redacted, changed, err := redactor.Redact(data)
	if err != nil {
		return "", err
	}
	if changed {
		if err := os.WriteFile(path, redacted, info.Mode().Perm()); err != nil {
			return "", fmt.Errorf("write redacted artifact: %w", err)
		}
	}
	return preHash, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
