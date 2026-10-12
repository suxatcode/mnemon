// Package oidctest is a small in-process OIDC provider for mnemon tests.
package oidctest

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// IDP serves discovery, JWKS, an authorization redirect, and a token endpoint.
type IDP struct {
	URL      string
	Issuer   string
	Key      *rsa.PrivateKey
	Kid      string
	Audience string

	srv     *httptest.Server
	mu      sync.Mutex
	pubs    map[string]*rsa.PublicKey
	codes   map[string]string
	refresh map[string]string
	groups  map[string][]string
	fetches int
	// NextAccessTTL overrides the access-token lifetime for the next token response.
	NextAccessTTL time.Duration
}

// New starts a loopback IdP. The issuer is the server URL.
func New(t *testing.T) *IDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &IDP{
		Key:      key,
		Kid:      "test-key",
		Audience: "mnemon",
		pubs:     map[string]*rsa.PublicKey{},
		codes:    map[string]string{},
		refresh:  map[string]string{},
		groups:   map[string][]string{},
	}
	p.pubs[p.Kid] = &key.PublicKey
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("/jwks", p.jwks)
	mux.HandleFunc("/authorize", p.authorize)
	mux.HandleFunc("/token", p.token)
	p.srv = httptest.NewServer(mux)
	p.URL = p.srv.URL
	p.Issuer = p.srv.URL
	t.Cleanup(p.srv.Close)
	return p
}

// SetGroups sets the groups claim for a principal issued by this IdP.
func (p *IDP) SetGroups(principal string, groups []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.groups[principal] = groups
}

// RotateKey adds a new signing key and uses it for later tokens.
func (p *IDP) RotateKey(t *testing.T, kid string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	p.Key = key
	p.Kid = kid
	p.pubs[kid] = &key.PublicKey
	p.mu.Unlock()
}

// HideKey removes a kid from the published JWKS until it is rotated back in.
func (p *IDP) HideKey(kid string) {
	p.mu.Lock()
	delete(p.pubs, kid)
	p.mu.Unlock()
}

// ShowKey publishes a previously hidden kid.
func (p *IDP) ShowKey(kid string, pub *rsa.PublicKey) {
	p.mu.Lock()
	p.pubs[kid] = pub
	p.mu.Unlock()
}

// Fetches is how many times JWKS was downloaded.
func (p *IDP) Fetches() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fetches
}

// Sign builds an access token. extra claims override defaults.
func (p *IDP) Sign(principal string, ttl time.Duration, extra jwt.MapClaims) string {
	if ttl == 0 {
		ttl = time.Hour
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":                p.Issuer,
		"aud":                p.Audience,
		"sub":                "stable-" + principal,
		"preferred_username": principal,
		"scope":              "openid profile",
		"iat":                now.Unix(),
		"exp":                now.Add(ttl).Unix(),
	}
	p.mu.Lock()
	if g, ok := p.groups[principal]; ok {
		arr := make([]any, len(g))
		for i, s := range g {
			arr[i] = s
		}
		claims["groups"] = arr
	}
	key := p.Key
	kid := p.Kid
	p.mu.Unlock()
	for k, v := range extra {
		if v == nil {
			delete(claims, k)
			continue
		}
		claims[k] = v
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		panic(err)
	}
	return signed
}

func (p *IDP) discovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{
		"issuer":                 p.Issuer,
		"authorization_endpoint": p.URL + "/authorize",
		"token_endpoint":         p.URL + "/token",
		"jwks_uri":               p.URL + "/jwks",
	})
}

func (p *IDP) jwks(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.fetches++
	keys := make([]map[string]string, 0, len(p.pubs))
	for kid, pub := range p.pubs {
		keys = append(keys, map[string]string{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": kid,
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		})
	}
	p.mu.Unlock()
	writeJSON(w, map[string]any{"keys": keys})
}

func (p *IDP) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || redirect.Scheme == "" {
		http.Error(w, "bad redirect", http.StatusBadRequest)
		return
	}
	principal := q.Get("login_hint")
	if principal == "" {
		principal = "alice"
	}
	code := random(18)
	p.mu.Lock()
	p.codes[code] = principal
	p.mu.Unlock()
	rq := redirect.Query()
	rq.Set("code", code)
	rq.Set("state", q.Get("state"))
	redirect.RawQuery = rq.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (p *IDP) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	var principal string
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		p.mu.Lock()
		principal = p.codes[r.Form.Get("code")]
		delete(p.codes, r.Form.Get("code"))
		p.mu.Unlock()
		if principal == "" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
	case "refresh_token":
		p.mu.Lock()
		principal = p.refresh[r.Form.Get("refresh_token")]
		p.mu.Unlock()
		if principal == "" {
			http.Error(w, "bad refresh", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "unsupported grant", http.StatusBadRequest)
		return
	}
	ttl := time.Hour
	p.mu.Lock()
	if p.NextAccessTTL > 0 {
		ttl = p.NextAccessTTL
		p.NextAccessTTL = 0
	}
	p.mu.Unlock()
	access := p.Sign(principal, ttl, nil)
	ref := random(24)
	p.mu.Lock()
	p.refresh[ref] = principal
	p.mu.Unlock()
	writeJSON(w, map[string]any{
		"access_token":  access,
		"refresh_token": ref,
		"token_type":    "Bearer",
		"expires_in":    int(ttl.Seconds()),
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func random(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}
