package ui

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestUIWritePathsImportOnlyFacade enforces the core guardrail: the cognition
// console's write paths (and every ui package file) reach governed state ONLY
// through the internal/app facade. No store, event log, or audit package may be
// imported directly — those writes must go through the facade so the domain
// event + audit.recorded + proposal audit_refs are always emitted. This is the
// focused, ui-scoped counterpart to the repo-wide ring guard.
func TestUIWritePathsImportOnlyFacade(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	uiDir := filepath.Dir(thisFile) // .../harness/internal/ui

	const facade = "github.com/mnemon-dev/mnemon/harness/internal/app"
	const uiPrefix = "github.com/mnemon-dev/mnemon/harness/internal/ui"
	const modPrefix = "github.com/mnemon-dev/mnemon/"

	fset := token.NewFileSet()
	var violations []string

	err := filepath.WalkDir(uiDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		rel, _ := filepath.Rel(uiDir, path)
		for _, spec := range f.Imports {
			imp := strings.Trim(spec.Path.Value, `"`)
			if !strings.HasPrefix(imp, modPrefix) {
				continue // stdlib or third-party (bubbletea/lipgloss) — allowed
			}
			if imp == facade || strings.HasPrefix(imp, uiPrefix) {
				continue // the facade, or a sibling ui/* package — allowed
			}
			violations = append(violations, rel+" -> "+imp)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ui tree: %v", err)
	}
	if len(violations) > 0 {
		t.Errorf("ui must import only the app facade (no store/eventlog/auditstore); offending imports:\n  %s",
			strings.Join(violations, "\n  "))
	}
}
