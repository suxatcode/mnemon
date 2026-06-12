package ui

import (
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
)

// P6c: the vocabulary lint (in the gates — it is an ordinary go test). The Control Tower speaks the
// sanctioned PRODUCT vocabulary (the four §3.3 page names + protocol terms), never foreign-discipline
// jargon (OA/OKR/kanban/sprint/dashboard) and never the non-existent "human@owner" identity (the
// operator is a control-agent — MUST-FIX 2). The check is over the Tower's STRUCTURAL vocabulary: a
// render with EMPTY data carries only the Tower's own labels, with no injected resource content.
func TestTowerVocabularyIsSanctioned(t *testing.T) {
	// the page names are EXACTLY the four sanctioned pages, in §3.3 order (a closed set).
	want := []string{"GOAL", "FIELD", "INBOX", "LEDGER"}
	if len(pageTitles) != len(want) {
		t.Fatalf("Tower must have exactly %d pages, got %d", len(want), len(pageTitles))
	}
	for i, w := range want {
		if pageTitles[i] != w {
			t.Fatalf("page %d title = %q, want %q", i, pageTitles[i], w)
		}
	}

	// the structural vocabulary must contain no foreign-discipline jargon and no non-existent identity.
	structural := strings.ToLower(NewTowerModel(app.TowerView{}).RenderAll())
	forbidden := []string{
		"okr", "kpi", "kanban", "看板", "任务看板", "sprint", "scrum",
		"dashboard", "backlog", "human@owner", "owner-kind",
	}
	for _, term := range forbidden {
		if strings.Contains(structural, strings.ToLower(term)) {
			t.Fatalf("Tower vocabulary must not contain the forbidden term %q:\n%s", term, structural)
		}
	}

	// positive: the sanctioned page titles ARE present in the structural render.
	for _, w := range want {
		if !strings.Contains(NewTowerModel(app.TowerView{}).RenderAll(), w) {
			t.Fatalf("structural render must name the sanctioned page %q", w)
		}
	}
}
