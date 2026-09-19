package remoteauth

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/suxatcode/mnemon/internal/model"
	"github.com/suxatcode/mnemon/internal/store"
)

func TestJWTValidExpiredRevokedAndForgedSub(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	key := []byte("0123456789abcdef0123456789abcdef")
	issuer := Issuer{DB: db, Key: key}
	v := StoreVerifier{DB: db, Key: key}

	token, ident, err := issuer.Issue("alice@team", model.RoleUser, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Verify(token)
	if err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if got.Principal != "alice@team" || got.Role != model.RoleUser {
		t.Fatalf("identity %+v", got)
	}

	if _, _, err := issuer.Issue("bob@team", model.RoleUser, time.Hour); err != nil {
		t.Fatal(err)
	}
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":  jwtIssuer,
		"sub":  "bob@team",
		"role": model.RoleUser,
		"jti":  ident.JTI,
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	forgedTok, err := forged.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(forgedTok); err == nil {
		t.Fatal("forged sub on another principal's jti must fail")
	}

	if err := db.UpsertPrincipal("carol@team", model.RoleUser); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-2 * time.Hour)
	expiredJTI := "expired-jti-test"
	if err := db.InsertIssuedToken(expiredJTI, "carol@team", past); err != nil {
		t.Fatal(err)
	}
	expiredTok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":  jwtIssuer,
		"sub":  "carol@team",
		"role": model.RoleUser,
		"jti":  expiredJTI,
		"iat":  past.Unix(),
		"exp":  past.Unix(),
	}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(expiredTok); err == nil {
		t.Fatal("expired token must fail")
	}

	live, liveIdent, err := issuer.Issue("dave@team", model.RoleUser, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeIssuedToken(liveIdent.JTI); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(live); err == nil {
		t.Fatal("revoked token must fail")
	}
}

func TestLoadKeyRejectsShort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jwt.key")
	if err := os.WriteFile(path, []byte("too-short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKey(path); err == nil {
		t.Fatal("short key must fail")
	}
}
