package remoteauth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/store"
)

const (
	jwtIssuer     = "mnemon-server"
	defaultExpiry = 90 * 24 * time.Hour
)

// Identity is the verified actor from a JWT. Principal always comes from sub.
type Identity struct {
	Principal string
	Role      string
	JTI       string
}

// Verifier validates a bearer token. Implementations can later be swapped for OIDC.
type Verifier interface {
	Verify(token string) (Identity, error)
}

// Issuer mints self-issued JWTs and persists jti rows.
type Issuer struct {
	DB  *store.DB
	Key []byte
}

// StoreVerifier checks signature, expiry, principal, and jti revocation against the DB.
type StoreVerifier struct {
	DB  *store.DB
	Key []byte
}

func (v StoreVerifier) Verify(token string) (Identity, error) {
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return v.Key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		return Identity{}, fmt.Errorf("invalid token")
	}
	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	jti, _ := claims["jti"].(string)
	if sub == "" || jti == "" {
		return Identity{}, fmt.Errorf("invalid token claims")
	}
	if role != model.RoleUser && role != model.RoleOrg {
		return Identity{}, fmt.Errorf("invalid token role")
	}
	tok, err := v.DB.LookupIssuedToken(jti)
	if err != nil {
		return Identity{}, fmt.Errorf("unknown token")
	}
	if tok.Revoked {
		return Identity{}, fmt.Errorf("token revoked")
	}
	if time.Now().UTC().After(tok.ExpiresAt) {
		return Identity{}, fmt.Errorf("token expired")
	}
	p, err := v.DB.GetPrincipal(sub)
	if err != nil {
		return Identity{}, fmt.Errorf("unknown principal")
	}
	if p.Disabled {
		return Identity{}, fmt.Errorf("principal disabled")
	}
	if tok.Principal != p.Principal {
		return Identity{}, fmt.Errorf("token principal mismatch")
	}
	return Identity{Principal: p.Principal, Role: p.Role, JTI: jti}, nil
}

// Issue creates a principal (if needed), records jti, and returns a signed JWT.
func (i Issuer) Issue(principal, role string, ttl time.Duration) (string, Identity, error) {
	if ttl <= 0 {
		ttl = defaultExpiry
	}
	if err := i.DB.UpsertPrincipal(principal, role); err != nil {
		return "", Identity{}, err
	}
	jti, err := randomJTI()
	if err != nil {
		return "", Identity{}, err
	}
	now := time.Now().UTC()
	exp := now.Add(ttl)
	if err := i.DB.InsertIssuedToken(jti, principal, exp); err != nil {
		return "", Identity{}, err
	}
	claims := jwt.MapClaims{
		"iss":  jwtIssuer,
		"sub":  principal,
		"role": role,
		"jti":  jti,
		"iat":  now.Unix(),
		"exp":  exp.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(i.Key)
	if err != nil {
		return "", Identity{}, err
	}
	return signed, Identity{Principal: principal, Role: role, JTI: jti}, nil
}

func randomJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// LoadKey reads a JWT HMAC key from a file. The file must be at least 32 bytes.
func LoadKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key := []byte(strings.TrimSpace(string(data)))
	if len(key) < 32 {
		return nil, fmt.Errorf("jwt key must be at least 32 bytes")
	}
	return key, nil
}

// GenerateKey returns a 32-byte random HMAC key as hex.
func GenerateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
