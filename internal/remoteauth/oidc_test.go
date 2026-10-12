package remoteauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/suxatcode/mnemon/internal/model"
	"github.com/suxatcode/mnemon/internal/oidctest"
	"github.com/suxatcode/mnemon/internal/store"
)

func testOIDCConfig(idp *oidctest.IDP) OIDCConfig {
	return OIDCConfig{
		Issuer:     idp.Issuer,
		Audience:   idp.Audience,
		AllowClaim: "groups",
		AllowValue: "mnemon-users",
		Provision:  ProvisionAuto,
	}
}

func TestOIDCAccessTokenAndRejections(t *testing.T) {
	idp := oidctest.New(t)
	db := openDB(t)
	v, err := NewOIDCVerifier(db, testOIDCConfig(idp))
	if err != nil {
		t.Fatal(err)
	}
	groups := jwt.MapClaims{"groups": []any{"mnemon-users"}}
	good := idp.Sign("alice", time.Hour, groups)
	ident, err := v.Verify(good)
	if err != nil {
		t.Fatalf("valid access token: %v", err)
	}
	if ident.Principal != "alice" || ident.Role != model.RoleUser {
		t.Fatalf("identity %+v", ident)
	}

	cases := []struct {
		name  string
		token string
	}{
		{"wrong aud", idp.Sign("alice", time.Hour, jwt.MapClaims{"aud": "other", "groups": []any{"mnemon-users"}})},
		{"wrong iss", idp.Sign("alice", time.Hour, jwt.MapClaims{"iss": "https://evil.example", "groups": []any{"mnemon-users"}})},
		{"expired", idp.Sign("alice", -2*time.Hour, groups)},
		{"missing principal", idp.Sign("alice", time.Hour, jwt.MapClaims{"preferred_username": nil, "groups": []any{"mnemon-users"}})},
		{"allow miss", idp.Sign("bob", time.Hour, jwt.MapClaims{"groups": []any{"other"}})},
		{"id token", idp.Sign("alice", time.Hour, jwt.MapClaims{"nonce": "n", "scope": nil, "groups": []any{"mnemon-users"}})},
		{"alg none", noneToken(t)},
	}
	for _, tc := range cases {
		if _, err := v.Verify(tc.token); err == nil {
			t.Fatalf("%s: expected rejection", tc.name)
		}
	}

	hmacTok := hs256Token(t, []byte("0123456789abcdef0123456789abcdef"))
	if _, err := v.Verify(hmacTok); err == nil {
		t.Fatal("HS256 token must not be accepted as OIDC")
	}
}

func TestOIDCOrgRoleDisabledAndAllowlist(t *testing.T) {
	idp := oidctest.New(t)
	db := openDB(t)
	cfg := testOIDCConfig(idp)
	cfg.RoleClaim = "groups"
	cfg.OrgValue = "org-admins"
	v, err := NewOIDCVerifier(db, cfg)
	if err != nil {
		t.Fatal(err)
	}
	orgTok := idp.Sign("ada", time.Hour, jwt.MapClaims{"groups": []any{"mnemon-users", "org-admins"}})
	ident, err := v.Verify(orgTok)
	if err != nil {
		t.Fatal(err)
	}
	if ident.Role != model.RoleOrg {
		t.Fatalf("role %s", ident.Role)
	}

	userTok := idp.Sign("cara", time.Hour, jwt.MapClaims{"groups": []any{"mnemon-users"}})
	if _, err := v.Verify(userTok); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPrincipalDisabled("cara", true); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(userTok); err == nil {
		t.Fatal("disabled principal must be rejected")
	}
	p, err := db.GetPrincipal("cara")
	if err != nil || !p.Disabled {
		t.Fatalf("principal should stay disabled: %+v err=%v", p, err)
	}

	allow, err := NewOIDCVerifier(db, OIDCConfig{
		Issuer: idp.Issuer, Audience: idp.Audience, Provision: ProvisionAllowlist,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allow.Verify(idp.Sign("nobody", time.Hour, nil)); err == nil {
		t.Fatal("allowlist must reject unknown principals")
	}
}

func TestOIDCUnknownKidRefetch(t *testing.T) {
	idp := oidctest.New(t)
	db := openDB(t)
	v, err := NewOIDCVerifier(db, testOIDCConfig(idp))
	if err != nil {
		t.Fatal(err)
	}
	first := idp.Sign("alice", time.Hour, jwt.MapClaims{"groups": []any{"mnemon-users"}})
	if _, err := v.Verify(first); err != nil {
		t.Fatal(err)
	}
	before := idp.Fetches()
	idp.RotateKey(t, "rotated")
	second := idp.Sign("alice", time.Hour, jwt.MapClaims{"groups": []any{"mnemon-users"}})
	if _, err := v.Verify(second); err != nil {
		t.Fatalf("refetch: %v", err)
	}
	if idp.Fetches() <= before {
		t.Fatalf("expected a JWKS refetch, fetches=%d before=%d", idp.Fetches(), before)
	}
}

func TestDispatchKeepsInviteJWT(t *testing.T) {
	idp := oidctest.New(t)
	db := openDB(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	inviteTok, _, err := (Issuer{DB: db, Key: key}).Issue("alice@team", model.RoleUser, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	oidc, err := NewOIDCVerifier(db, testOIDCConfig(idp))
	if err != nil {
		t.Fatal(err)
	}
	d := DispatchVerifier{Invite: &StoreVerifier{DB: db, Key: key}, OIDC: oidc}
	got, err := d.Verify(inviteTok)
	if err != nil || got.Principal != "alice@team" {
		t.Fatalf("invite via dispatch: %+v %v", got, err)
	}
	access := idp.Sign("alice", time.Hour, jwt.MapClaims{"groups": []any{"mnemon-users"}})
	got, err = d.Verify(access)
	if err != nil || got.Principal != "alice" {
		t.Fatalf("oidc via dispatch: %+v %v", got, err)
	}
	if _, err := (&StoreVerifier{DB: db, Key: key}).Verify(access); err == nil {
		t.Fatal("invite verifier must not accept an OIDC token")
	}
}

func TestOIDCConfigFailClosed(t *testing.T) {
	cfg := OIDCConfig{Issuer: "https://idp.example", Audience: "mnemon", Provision: ProvisionAuto}
	if err := cfg.Prepare(); err == nil {
		t.Fatal("auto provision without an allow claim must fail")
	}
	cfg.Provision = ProvisionAllowlist
	if err := cfg.Prepare(); err != nil {
		t.Fatal(err)
	}
	db := openDB(t)
	if _, err := NewVerifier(db, "", OIDCConfig{}); err == nil {
		t.Fatal("neither jwt key nor oidc issuer must fail")
	}
}

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func noneToken(t *testing.T) string {
	t.Helper()
	header := b64([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := b64([]byte(`{"iss":"x","aud":"mnemon","preferred_username":"alice"}`))
	return header + "." + payload + "."
}

func hs256Token(t *testing.T, key []byte) string {
	t.Helper()
	header := b64([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, _ := json.Marshal(map[string]any{
		"iss": "mnemon-server", "aud": "mnemon", "preferred_username": "alice",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	payload := b64(body)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(header + "." + payload))
	return header + "." + payload + "." + b64(mac.Sum(nil))
}

func b64(b []byte) string {
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(b), "=")
}
