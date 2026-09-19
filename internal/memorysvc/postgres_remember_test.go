package memorysvc

import (
	"os"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/store"
)

func TestPostgresRememberDoesNotRollback(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("TEST_POSTGRES_URL"))
	if url == "" {
		t.Skip("TEST_POSTGRES_URL not set")
	}
	db, err := store.OpenWithOptions(store.Options{DatabaseURL: url})
	if err != nil {
		if strings.Contains(err.Error(), "connect") || strings.Contains(err.Error(), "dial") {
			t.Skipf("TEST_POSTGRES_URL unreachable: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := New(db, Options{MaxInsights: 100, EnforceACL: true, StoreName: "default"})
	res, err := svc.Remember(Actor{Principal: "alice@team", Role: model.RoleUser, Agent: "test"}, RememberInput{
		Content: "alice-only memory about widgets", Category: "fact", Importance: 3, NoDiff: true,
	})
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	if !strings.Contains(string(res.JSON), `"owner_principal": "alice@team"`) {
		t.Fatalf("json: %s", res.JSON)
	}
	org, err := svc.Remember(Actor{Principal: "org@team", Role: model.RoleOrg, Agent: "test"}, RememberInput{
		Content: "org-wide policy: use go 1.24", Category: "fact", Importance: 4, NoDiff: true,
	})
	if err != nil {
		t.Fatalf("org remember (entity extract): %v", err)
	}
	if !strings.Contains(string(org.JSON), `"layer": "org"`) {
		t.Fatalf("org json: %s", org.JSON)
	}
}
