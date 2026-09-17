package memorysvc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/store"
)

func TestPersonalPruneDoesNotDeleteOrg(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := New(db, Options{EnforceACL: true, MaxInsights: 5, StoreName: "default"})
	alice := Actor{Principal: "alice", Role: model.RoleUser, Agent: "test"}
	org := Actor{Principal: "organization", Role: model.RoleOrg, Agent: "test"}

	orgRes, err := svc.Remember(org, RememberInput{Content: "org ground truth must survive personal prune", Category: "decision", Importance: 5, NoDiff: true})
	if err != nil {
		t.Fatal(err)
	}
	var orgOut struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(orgRes.JSON, &orgOut); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 8; i++ {
		content := "alice disposable note number " + strings.Repeat("x", i+1)
		if _, err := svc.Remember(alice, RememberInput{Content: content, Category: "fact", Importance: 1, NoDiff: true}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.GetInsightByID(orgOut.ID)
	if err != nil || got == nil {
		t.Fatal("org memory was pruned or missing")
	}
}

func TestUserCannotForgetOtherPersonal(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := New(db, Options{EnforceACL: true, MaxInsights: 100})
	alice := Actor{Principal: "alice", Role: model.RoleUser, Agent: "t"}
	bob := Actor{Principal: "bob", Role: model.RoleUser, Agent: "t"}
	res, err := svc.Remember(alice, RememberInput{Content: "alice only", Category: "fact", Importance: 3, NoDiff: true})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.JSON, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Forget(bob, ForgetInput{ID: out.ID}); err == nil {
		t.Fatal("bob must not forget alice")
	}
	if _, err := svc.Forget(alice, ForgetInput{ID: out.ID}); err != nil {
		t.Fatal(err)
	}
}
