package remoteauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
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
	var authURL string
	remote, err := LoginOIDC(context.Background(), OIDCLoginRequest{
		Issuer:   idp.Issuer,
		ClientID: "mnemon-cli",
		Server:   "http://127.0.0.1:9",
		Name:     "team",
		DataDir:  dir,
		Timeout:  10 * time.Second,
		OpenBrowser: func(raw string) error {
			authURL = raw
			return followLogin(t)(raw)
		},
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
	scope := scopeParam(t, authURL)
	if !strings.Contains(scope, "openid") || !strings.Contains(scope, "profile") {
		t.Fatalf("default scope %q", scope)
	}
	if strings.Contains(scope, "offline_access") {
		t.Fatalf("default login must not request offline_access: %q", scope)
	}
}

func TestLoginOIDCExtraScopes(t *testing.T) {
	idp := oidctest.New(t)
	idp.SetGroups("alice", []string{"mnemon-users"})
	var authURL string
	_, err := LoginOIDC(context.Background(), OIDCLoginRequest{
		Issuer:   idp.Issuer,
		ClientID: "mnemon-cli",
		Server:   "http://127.0.0.1:9",
		DataDir:  t.TempDir(),
		Timeout:  10 * time.Second,
		Scopes:   []string{"api://example/access", "offline_access", "openid", "  "},
		OpenBrowser: func(raw string) error {
			authURL = raw
			return followLogin(t)(raw)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	scope := scopeParam(t, authURL)
	for _, want := range []string{"openid", "profile", "api://example/access", "offline_access"} {
		if !strings.Contains(scope, want) {
			t.Fatalf("scope %q missing %q", scope, want)
		}
	}
	if strings.Count(scope, "openid") != 1 {
		t.Fatalf("openid duplicated in %q", scope)
	}
}

func scopeParam(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("scope")
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
