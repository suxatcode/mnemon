package remoteauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/suxatcode/mnemon/internal/model"
	"github.com/suxatcode/mnemon/internal/store"
)

const (
	// ProvisionAuto inserts a user principal the first time a valid token is seen.
	ProvisionAuto = "auto"
	// ProvisionAllowlist rejects principals that were not created ahead of time.
	ProvisionAllowlist = "allowlist"

	defaultPrincipalClaim = "preferred_username"
	oidcSkew              = 60 * time.Second
)

// OIDCConfig is generic OIDC access-token verification. It does not name a vendor.
type OIDCConfig struct {
	Issuer         string
	Audience       string
	JWKSURL        string
	PrincipalClaim string
	AllowClaim     string
	AllowValue     string
	RoleClaim      string
	OrgValue       string
	Provision      string
}

// Prepare applies defaults and rejects a configuration that would admit every
// account at the issuer. A claim gate or an allowlist is required.
func (c *OIDCConfig) Prepare() error {
	if c.Issuer == "" {
		return fmt.Errorf("oidc issuer is required")
	}
	c.Issuer = strings.TrimRight(c.Issuer, "/")
	if c.Audience == "" {
		return fmt.Errorf("oidc audience is required")
	}
	if c.PrincipalClaim == "" {
		c.PrincipalClaim = defaultPrincipalClaim
	}
	if c.Provision == "" {
		c.Provision = ProvisionAuto
	}
	if c.Provision != ProvisionAuto && c.Provision != ProvisionAllowlist {
		return fmt.Errorf("oidc provision must be %s or %s", ProvisionAuto, ProvisionAllowlist)
	}
	if (c.AllowClaim == "") != (c.AllowValue == "") {
		return fmt.Errorf("oidc allow claim and allow value must both be set")
	}
	if (c.RoleClaim == "") != (c.OrgValue == "") {
		return fmt.Errorf("oidc role claim and org value must both be set")
	}
	if c.Provision == ProvisionAuto && c.AllowClaim == "" {
		return fmt.Errorf("oidc auto provision requires an allow claim, or set provision=%s", ProvisionAllowlist)
	}
	return nil
}

// OIDCVerifier checks RS256/ES256 access tokens against an IdP JWKS.
type OIDCVerifier struct {
	DB     *store.DB
	Config OIDCConfig
	HTTP   *http.Client

	mu      sync.Mutex
	keys    map[string]any
	jwksURL string
}

// NewOIDCVerifier validates cfg and returns a verifier. JWKS is fetched on first use.
func NewOIDCVerifier(db *store.DB, cfg OIDCConfig) (*OIDCVerifier, error) {
	if err := cfg.Prepare(); err != nil {
		return nil, err
	}
	return &OIDCVerifier{
		DB:     db,
		Config: cfg,
		HTTP:   &http.Client{Timeout: 10 * time.Second},
		keys:   map[string]any{},
	}, nil
}

// Verify validates an OIDC access token and resolves the local principal.
func (v *OIDCVerifier) Verify(token string) (Identity, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, v.keyfunc,
		jwt.WithValidMethods([]string{
			jwt.SigningMethodRS256.Alg(),
			jwt.SigningMethodES256.Alg(),
		}),
		jwt.WithIssuer(v.Config.Issuer),
		jwt.WithAudience(v.Config.Audience),
		jwt.WithLeeway(oidcSkew),
	)
	if err != nil || !parsed.Valid {
		return Identity{}, fmt.Errorf("invalid token")
	}
	if isIDToken(claims) {
		return Identity{}, fmt.Errorf("id token rejected")
	}
	principal, _ := claims[v.Config.PrincipalClaim].(string)
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return Identity{}, fmt.Errorf("missing principal claim %q", v.Config.PrincipalClaim)
	}
	if v.Config.AllowClaim != "" && !claimHas(claims, v.Config.AllowClaim, v.Config.AllowValue) {
		return Identity{}, fmt.Errorf("allow claim mismatch")
	}
	role := model.RoleUser
	if v.Config.RoleClaim != "" && claimHas(claims, v.Config.RoleClaim, v.Config.OrgValue) {
		role = model.RoleOrg
	}
	return v.resolvePrincipal(principal, role)
}

func isIDToken(claims jwt.MapClaims) bool {
	if _, ok := claims["nonce"]; !ok {
		return false
	}
	_, scope := claims["scope"]
	_, scp := claims["scp"]
	return !scope && !scp
}

func (v *OIDCVerifier) resolvePrincipal(principal, role string) (Identity, error) {
	existing, err := v.DB.GetPrincipal(principal)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return Identity{}, err
		}
		if v.Config.Provision == ProvisionAllowlist {
			return Identity{}, fmt.Errorf("unknown principal")
		}
		if err := v.DB.UpsertPrincipal(principal, role); err != nil {
			return Identity{}, err
		}
		return Identity{Principal: principal, Role: role}, nil
	}
	if existing.Disabled {
		return Identity{}, fmt.Errorf("principal disabled")
	}
	if role == model.RoleOrg && existing.Role != model.RoleOrg {
		if err := v.DB.UpsertPrincipal(principal, role); err != nil {
			return Identity{}, err
		}
		return Identity{Principal: principal, Role: role}, nil
	}
	return Identity{Principal: existing.Principal, Role: existing.Role}, nil
}

func (v *OIDCVerifier) keyfunc(t *jwt.Token) (any, error) {
	alg := ""
	if t.Method != nil {
		alg = t.Method.Alg()
	}
	if alg != jwt.SigningMethodRS256.Alg() && alg != jwt.SigningMethodES256.Alg() {
		return nil, fmt.Errorf("unexpected signing method")
	}
	kid, _ := t.Header["kid"].(string)
	if kid == "" {
		return nil, fmt.Errorf("missing kid")
	}
	if key, ok := v.cached(kid); ok {
		return key, nil
	}
	if err := v.refresh(context.Background()); err != nil {
		return nil, err
	}
	if key, ok := v.cached(kid); ok {
		return key, nil
	}
	return nil, fmt.Errorf("unknown kid")
}

func (v *OIDCVerifier) cached(kid string) (any, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key, ok := v.keys[kid]
	return key, ok
}

func (v *OIDCVerifier) refresh(ctx context.Context) error {
	url, err := v.jwksEndpoint(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := v.http().Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks: http %d", resp.StatusCode)
	}
	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}
	keys := make(map[string]any, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Use == "enc" || k.Kid == "" {
			continue
		}
		pub, err := k.publicKey()
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}
	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

func (v *OIDCVerifier) jwksEndpoint(ctx context.Context) (string, error) {
	v.mu.Lock()
	cached := v.jwksURL
	v.mu.Unlock()
	if cached != "" {
		return cached, nil
	}
	if v.Config.JWKSURL != "" {
		v.mu.Lock()
		v.jwksURL = v.Config.JWKSURL
		v.mu.Unlock()
		return v.Config.JWKSURL, nil
	}
	disc, err := Discover(ctx, v.http(), v.Config.Issuer)
	if err != nil {
		return "", err
	}
	if disc.JWKSURI == "" {
		return "", fmt.Errorf("discovery response has no jwks_uri")
	}
	v.mu.Lock()
	v.jwksURL = disc.JWKSURI
	v.mu.Unlock()
	return disc.JWKSURI, nil
}

func (v *OIDCVerifier) http() *http.Client {
	if v.HTTP != nil {
		return v.HTTP
	}
	return http.DefaultClient
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (k jwk) publicKey() (any, error) {
	switch k.Kty {
	case "RSA":
		return rsaPublic(k.N, k.E)
	case "EC":
		if k.Crv != "" && k.Crv != "P-256" {
			return nil, fmt.Errorf("unsupported curve %s", k.Crv)
		}
		return ecdsaPublic(k.X, k.Y)
	default:
		return nil, fmt.Errorf("unsupported kty %s", k.Kty)
	}
}

func rsaPublic(nB64, eB64 string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	if len(eb) == 0 || len(nb) == 0 {
		return nil, fmt.Errorf("incomplete rsa jwk")
	}
	e := 0
	for _, b := range eb {
		e = e<<8 | int(b)
	}
	if e < 2 {
		return nil, fmt.Errorf("invalid rsa exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: e}, nil
}

func ecdsaPublic(xB64, yB64 string) (*ecdsa.PublicKey, error) {
	xb, err := base64.RawURLEncoding.DecodeString(xB64)
	if err != nil {
		return nil, err
	}
	yb, err := base64.RawURLEncoding.DecodeString(yB64)
	if err != nil {
		return nil, err
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xb),
		Y:     new(big.Int).SetBytes(yb),
	}, nil
}

func claimHas(claims jwt.MapClaims, name, want string) bool {
	raw, ok := claims[name]
	if !ok || want == "" {
		return false
	}
	switch v := raw.(type) {
	case string:
		if v == want {
			return true
		}
		for _, part := range strings.FieldsFunc(v, func(r rune) bool {
			return r == ' ' || r == ','
		}) {
			if part == want {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

// DispatchVerifier routes HS256 invite tokens and RS256/ES256 OIDC tokens.
// The HMAC key is never used on an OIDC token.
type DispatchVerifier struct {
	Invite *StoreVerifier
	OIDC   *OIDCVerifier
}

// Verify implements Verifier.
func (d DispatchVerifier) Verify(token string) (Identity, error) {
	alg, err := tokenAlg(token)
	if err != nil {
		return Identity{}, fmt.Errorf("invalid token")
	}
	switch alg {
	case jwt.SigningMethodHS256.Alg():
		if d.Invite == nil {
			return Identity{}, fmt.Errorf("invite auth is not configured")
		}
		return d.Invite.Verify(token)
	case jwt.SigningMethodRS256.Alg(), jwt.SigningMethodES256.Alg():
		if d.OIDC == nil {
			return Identity{}, fmt.Errorf("oidc is not configured")
		}
		return d.OIDC.Verify(token)
	default:
		return Identity{}, fmt.Errorf("unsupported signing method")
	}
}

func tokenAlg(token string) (string, error) {
	parser := jwt.NewParser()
	parsed, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil || parsed.Method == nil {
		return "", fmt.Errorf("invalid token header")
	}
	return parsed.Method.Alg(), nil
}

// NewVerifier builds the invite verifier, the OIDC verifier, or both.
// jwtKeyFile may be empty only when cfg.Issuer is set.
func NewVerifier(db *store.DB, jwtKeyFile string, cfg OIDCConfig) (Verifier, error) {
	var invite *StoreVerifier
	if jwtKeyFile != "" {
		key, err := LoadKey(jwtKeyFile)
		if err != nil {
			return nil, fmt.Errorf("jwt key: %w", err)
		}
		invite = &StoreVerifier{DB: db, Key: key}
	}
	var oidc *OIDCVerifier
	if cfg.Issuer != "" {
		v, err := NewOIDCVerifier(db, cfg)
		if err != nil {
			return nil, err
		}
		oidc = v
	}
	if invite == nil && oidc == nil {
		return nil, fmt.Errorf("configure --jwt-key or --oidc-issuer")
	}
	if oidc == nil {
		return *invite, nil
	}
	if invite == nil {
		return DispatchVerifier{OIDC: oidc}, nil
	}
	return DispatchVerifier{Invite: invite, OIDC: oidc}, nil
}
