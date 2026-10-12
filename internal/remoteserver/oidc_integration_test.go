package remoteserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/suxatcode/mnemon/internal/memorysvc"
	"github.com/suxatcode/mnemon/internal/model"
	"github.com/suxatcode/mnemon/internal/oidctest"
	"github.com/suxatcode/mnemon/internal/remoteapi"
	"github.com/suxatcode/mnemon/internal/remoteauth"
	"github.com/suxatcode/mnemon/internal/remoteclient"
	"github.com/suxatcode/mnemon/internal/store"
)

func TestOIDCAndInviteOnTheSameGateway(t *testing.T) {
	idp := oidctest.New(t)
	idp.SetGroups("alice", []string{"mnemon-users"})
	idp.SetGroups("bob", []string{"mnemon-users"})
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	keyPath := filepath.Join(t.TempDir(), "jwt.key")
	if err := os.WriteFile(keyPath, []byte("0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	verifier, err := remoteauth.NewVerifier(db, keyPath, remoteauth.OIDCConfig{
		Issuer:     idp.Issuer,
		Audience:   idp.Audience,
		AllowClaim: "groups",
		AllowValue: "mnemon-users",
		Provision:  remoteauth.ProvisionAuto,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(memorysvc.New(db, memorysvc.Options{EnforceACL: true}), verifier, db).Handler()
	api := httptest.NewServer(handler)
	t.Cleanup(api.Close)

	dir := t.TempDir()
	remote, err := remoteauth.LoginOIDC(context.Background(), remoteauth.OIDCLoginRequest{
		Issuer:      idp.Issuer,
		ClientID:    "mnemon-cli",
		Server:      api.URL,
		Name:        "team",
		DataDir:     dir,
		MakeDefault: true,
		Timeout:     10 * time.Second,
		OpenBrowser: func(authURL string) error {
			var last error
			for i := 0; i < 20; i++ {
				resp, err := http.Get(authURL)
				if err != nil {
					last = err
				} else {
					resp.Body.Close()
					if resp.StatusCode < 400 {
						return nil
					}
					last = fmt.Errorf("status %s", resp.Status)
				}
				time.Sleep(50 * time.Millisecond)
			}
			if last == nil {
				return context.DeadlineExceeded
			}
			return last
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	alice, err := remoteclient.Dial(remote)
	if err != nil {
		t.Fatal(err)
	}
	defer alice.Close()
	remembered, err := alice.Remember(remoteapi.RememberRequest{
		Content: "alice oidc note about widgets", Category: "fact", Importance: 3, NoDiff: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if remembered == nil || len(remembered.JSON) == 0 {
		t.Fatal("expected remember json")
	}
	recalled, err := alice.Recall(remoteapi.RecallRequest{Query: "widgets", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(recalled.JSON), "alice oidc note about widgets") {
		t.Fatalf("recall: %s", recalled.JSON)
	}

	bobDir := t.TempDir()
	bobTok := idp.Sign("bob", time.Hour, jwt.MapClaims{"groups": []any{"mnemon-users"}})
	bobRemote := writeBearer(t, bobDir, api.URL, "bob", bobTok)
	bob, err := remoteclient.Dial(bobRemote)
	if err != nil {
		t.Fatal(err)
	}
	defer bob.Close()
	if _, err := bob.Remember(remoteapi.RememberRequest{
		Content: "bob oidc note about sprockets", Category: "fact", Importance: 3, NoDiff: true,
	}); err != nil {
		t.Fatal(err)
	}
	bobRecall, err := bob.Recall(remoteapi.RecallRequest{Query: "widgets", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bobRecall.JSON), "alice oidc note about widgets") {
		t.Fatal("team recall should see alice")
	}

	outsider := idp.Sign("mallory", time.Hour, jwt.MapClaims{"groups": []any{"other"}})
	outRemote := writeBearer(t, t.TempDir(), api.URL, "mallory", outsider)
	out, err := remoteclient.Dial(outRemote)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := out.Status(); err == nil {
		t.Fatal("token outside the allow claim must be rejected")
	}

	invite, _, err := (remoteauth.Issuer{DB: db, Key: []byte("0123456789abcdef0123456789abcdef")}).Issue("carol@team", model.RoleUser, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	invRemote := writeBearer(t, t.TempDir(), api.URL, "carol@team", invite)
	inv, err := remoteclient.Dial(invRemote)
	if err != nil {
		t.Fatal(err)
	}
	defer inv.Close()
	if _, err := inv.Remember(remoteapi.RememberRequest{
		Content: "invite still works", Category: "fact", Importance: 3, NoDiff: true,
	}); err != nil {
		t.Fatalf("invite jwt: %v", err)
	}

	if err := db.SetPrincipalDisabled("alice", true); err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Status(); err == nil {
		t.Fatal("revoked oidc principal must be rejected while the access token is otherwise valid")
	}
}

func writeBearer(t *testing.T, dir, server, principal, token string) remoteapi.RemoteConfig {
	t.Helper()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return remoteapi.RemoteConfig{Name: "t", Server: server, Principal: principal, TokenFile: path}
}
