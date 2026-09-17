package store

import (
	"fmt"
	"time"
)

// Principal is a durable identity stored alongside memory data.
type Principal struct {
	Principal string
	Role      string
	Disabled  bool
	CreatedAt time.Time
}

// IssuedToken is a revocable JWT identifier.
type IssuedToken struct {
	JTI       string
	Principal string
	ExpiresAt time.Time
	Revoked   bool
	CreatedAt time.Time
}

// UpsertPrincipal creates or updates a principal. Existing role is replaced.
func (db *DB) UpsertPrincipal(principal, role string) error {
	if principal == "" {
		return fmt.Errorf("principal is required")
	}
	if role != "user" && role != "org" {
		return fmt.Errorf("role must be user or org")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if db.dialect == DialectPostgres {
		_, err := db.execer().Exec(
			`INSERT INTO principals (principal, role, disabled, created_at)
			 VALUES (?, ?, 0, ?)
			 ON CONFLICT (principal) DO UPDATE SET role = EXCLUDED.role, disabled = 0`,
			principal, role, now)
		return err
	}
	_, err := db.execer().Exec(
		`INSERT INTO principals (principal, role, disabled, created_at)
		 VALUES (?, ?, 0, ?)
		 ON CONFLICT (principal) DO UPDATE SET role = excluded.role, disabled = 0`,
		principal, role, now)
	return err
}

// GetPrincipal loads a principal row.
func (db *DB) GetPrincipal(principal string) (*Principal, error) {
	var p Principal
	var disabled int
	var created string
	err := db.execer().QueryRow(
		`SELECT principal, role, disabled, created_at FROM principals WHERE principal = ?`,
		principal,
	).Scan(&p.Principal, &p.Role, &disabled, &created)
	if err != nil {
		return nil, err
	}
	p.Disabled = disabled != 0
	if p.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
		return nil, err
	}
	return &p, nil
}

// SetPrincipalDisabled toggles a principal without deleting history.
func (db *DB) SetPrincipalDisabled(principal string, disabled bool) error {
	flag := 0
	if disabled {
		flag = 1
	}
	res, err := db.execer().Exec(`UPDATE principals SET disabled = ? WHERE principal = ?`, flag, principal)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("unknown principal %q", principal)
	}
	return nil
}

// InsertIssuedToken records a newly issued JWT id.
func (db *DB) InsertIssuedToken(jti, principal string, expiresAt time.Time) error {
	_, err := db.execer().Exec(
		`INSERT INTO issued_tokens (jti, principal, expires_at, revoked, created_at)
		 VALUES (?, ?, ?, 0, ?)`,
		jti, principal, expiresAt.UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	return err
}

// LookupIssuedToken returns a token row by jti.
func (db *DB) LookupIssuedToken(jti string) (*IssuedToken, error) {
	var tok IssuedToken
	var expires, created string
	var revoked int
	err := db.execer().QueryRow(
		`SELECT jti, principal, expires_at, revoked, created_at FROM issued_tokens WHERE jti = ?`,
		jti,
	).Scan(&tok.JTI, &tok.Principal, &expires, &revoked, &created)
	if err != nil {
		return nil, err
	}
	tok.Revoked = revoked != 0
	if tok.ExpiresAt, err = time.Parse(time.RFC3339, expires); err != nil {
		return nil, err
	}
	if tok.CreatedAt, err = time.Parse(time.RFC3339, created); err != nil {
		return nil, err
	}
	return &tok, nil
}

// RevokeIssuedToken flags a jti as revoked.
func (db *DB) RevokeIssuedToken(jti string) error {
	res, err := db.execer().Exec(`UPDATE issued_tokens SET revoked = 1 WHERE jti = ?`, jti)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("unknown token %q", jti)
	}
	return nil
}

// RevokePrincipalTokens revokes every token for a principal.
func (db *DB) RevokePrincipalTokens(principal string) error {
	_, err := db.execer().Exec(`UPDATE issued_tokens SET revoked = 1 WHERE principal = ?`, principal)
	return err
}
