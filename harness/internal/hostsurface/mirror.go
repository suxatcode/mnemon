package hostsurface

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/suxatcode/mnemon/harness/internal/projection"
)

func WriteMemoryMirror(path string, proj projection.Projection) error {
	content := strings.TrimSpace(scopedMemoryContent(proj))
	if content == "" {
		content = "# Local Memory\n\n_No scoped memory entries._"
	}
	body := "# MEMORY.md\n\n" +
		"<!-- Non-authoritative mirror generated from Local Mnemon scoped memory. Do not edit directly; use memory-set. -->\n\n" +
		content + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Atomic AND concurrent-safe: the prime hook (another process) and the background driver may
	// regenerate this derived view simultaneously, so each writer gets its OWN temp file — a fixed
	// temp name would let writer B truncate the inode writer A is about to rename into place,
	// exposing B's half-written bytes through the target path. With per-writer temps the POSIX
	// rename is atomic: a reader sees either the old mirror or a complete new one, never a torn
	// one, and last-rename-wins between complete bodies.
	dir, base := filepath.Dir(path), filepath.Base(path)
	tmp, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename; cleans up on any failure path
	if _, err := tmp.WriteString(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { // CreateTemp creates 0600
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func scopedMemoryContent(proj projection.Projection) string {
	for _, item := range proj.Content {
		if item.Ref.Kind != "memory" {
			continue
		}
		if content, ok := item.Fields["content"].(string); ok {
			return content
		}
	}
	return ""
}
