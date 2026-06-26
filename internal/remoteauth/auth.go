package remoteauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/internal/remoteapi"
)

const HashPrefix = "sha256:"

type User struct {
	Principal string   `json:"principal"`
	TokenHash string   `json:"token_hash"`
	Scopes    []string `json:"scopes,omitempty"`
}

type UsersFile struct {
	SchemaVersion int    `json:"schema_version"`
	Users         []User `json:"users"`
}

func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "mnemon_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return HashPrefix + hex.EncodeToString(sum[:])
}

func VerifyToken(token, hash string) bool {
	if !strings.HasPrefix(hash, HashPrefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(hash, HashPrefix))
	if err != nil {
		return false
	}
	got := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(got[:], want) == 1
}

func LoadUsers(path string) (*UsersFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &UsersFile{SchemaVersion: 1}, nil
		}
		return nil, err
	}
	var doc UsersFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported users schema_version %d", doc.SchemaVersion)
	}
	return &doc, nil
}

func SaveUsers(path string, doc *UsersFile) error {
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func UpsertUser(doc *UsersFile, user User) {
	for i := range doc.Users {
		if doc.Users[i].Principal == user.Principal {
			doc.Users[i] = user
			return
		}
	}
	doc.Users = append(doc.Users, user)
}

func Authenticate(doc *UsersFile, principal, token string) error {
	if principal == "" || token == "" {
		return fmt.Errorf("missing principal or token")
	}
	for _, user := range doc.Users {
		if user.Principal == principal {
			if VerifyToken(token, user.TokenHash) {
				return nil
			}
			return fmt.Errorf("invalid token for principal %q", principal)
		}
	}
	return fmt.Errorf("unknown principal %q", principal)
}

func LoadAuthConfig(path string) (*remoteapi.AuthConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &remoteapi.AuthConfig{SchemaVersion: 1}, nil
		}
		return nil, err
	}
	var cfg remoteapi.AuthConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported auth schema_version %d", cfg.SchemaVersion)
	}
	return &cfg, nil
}

func SaveAuthConfig(path string, cfg *remoteapi.AuthConfig) error {
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func UpsertRemote(cfg *remoteapi.AuthConfig, remote remoteapi.RemoteConfig) {
	for i := range cfg.Remotes {
		if cfg.Remotes[i].Name == remote.Name {
			cfg.Remotes[i] = remote
			return
		}
	}
	cfg.Remotes = append(cfg.Remotes, remote)
}

func FindDefaultRemote(cfg *remoteapi.AuthConfig) (*remoteapi.RemoteConfig, bool) {
	if cfg.DefaultRemote == "" {
		return nil, false
	}
	for i := range cfg.Remotes {
		if cfg.Remotes[i].Name == cfg.DefaultRemote {
			return &cfg.Remotes[i], true
		}
	}
	return nil, false
}
