package memorysvc

import (
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/store"
)

func TestRememberRejectedWhenReadOnly(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	ro, err := store.OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ro.Close() })
	svc := New(ro, Options{MaxInsights: 100, StoreName: "default"})
	_, err = svc.Remember(Actor{Principal: model.LocalOwner, Role: model.RoleUser}, RememberInput{
		Content: "should not write", Category: "fact", Importance: 3, NoDiff: true,
	})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("want read-only error, got %v", err)
	}
}
