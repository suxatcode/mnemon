package remoteauth

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/suxatcode/mnemon/internal/oidctest"
	"github.com/suxatcode/mnemon/internal/remoteapi"
)

func TestLoginOIDCAndRefresh(t *testing.T) {
	idp := oidctest.New(t)
	idp.SetGroups("alice", []string{"mnemon-users"})
	idp.NextAccessTTL = 30 * time.Second
	dir := t.TempDir()
	remote, err := LoginOIDC(context.Background(), OIDCLoginRequest{
		Issuer:      idp.Issuer,
		ClientID:    "mnemon-cli",
		Server:      "http://127.0.0.1:9",
		Name:        "team",
		DataDir:     dir,
		Timeout:     10 * time.Second,
		OpenBrowser: followLogin(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if remote.Auth != "oidc" || remote.Principal != "alice" || remote.RefreshFile == "" {
		t.Fatalf("remote %+v", remote)
	}
	if remote.Issuer != idp.Issuer {
		t.Fatalf("issuer %s", remote.Issuer)
	}
	fresh, err := FreshAccessToken(context.Background(), remote)
	if err != nil {
		t.Fatal(err)
	}
	if fresh == "" || !isJWT(fresh) {
		t.Fatal("expected a refreshed jwt")
	}
	cfg, err := LoadAuthConfig(filepath.Join(dir, remoteapi.DefaultAuthFileName))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultRemote != "" {
		t.Fatal("login without MakeDefault must not set the default remote")
	}
}

func followLogin(t *testing.T) func(string) error {
	t.Helper()
	return func(authURL string) error {
		var last error
		for i := 0; i < 20; i++ {
			resp, err := http.Get(authURL)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode < 400 {
					return nil
				}
				last = fmt.Errorf("status %s", resp.Status)
			} else {
				last = err
			}
			time.Sleep(50 * time.Millisecond)
		}
		return last
	}
}
