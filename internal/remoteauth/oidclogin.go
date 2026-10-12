package remoteauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/suxatcode/mnemon/internal/remoteapi"
	"golang.org/x/oauth2"
)

const accessRefreshSkew = 60 * time.Second

// Discovery is the subset of an OpenID provider metadata document the CLI and
// the server both need.
type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// Discover loads the issuer's OpenID metadata.
func Discover(ctx context.Context, client *http.Client, issuer string) (Discovery, error) {
	if client == nil {
		client = http.DefaultClient
	}
	issuer = strings.TrimRight(issuer, "/")
	url := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Discovery{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Discovery{}, fmt.Errorf("oidc discovery: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Discovery{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Discovery{}, fmt.Errorf("oidc discovery: http %d", resp.StatusCode)
	}
	var doc Discovery
	if err := json.Unmarshal(body, &doc); err != nil {
		return Discovery{}, fmt.Errorf("oidc discovery: %w", err)
	}
	if doc.TokenEndpoint == "" || doc.AuthorizationEndpoint == "" {
		return Discovery{}, fmt.Errorf("oidc discovery missing endpoints")
	}
	return doc, nil
}

// OIDCLoginRequest is a public-client authorization-code login with PKCE.
type OIDCLoginRequest struct {
	Issuer         string
	ClientID       string
	Server         string
	Name           string
	DataDir        string
	PrincipalClaim string
	CAFile         string
	ServerName     string
	MakeDefault    bool
	// OpenBrowser is called with the authorization URL. Nil prints the URL and
	// tries the OS opener. Tests can complete the redirect themselves.
	OpenBrowser func(url string) error
	Timeout     time.Duration
	HTTP        *http.Client
}

// LoginOIDC performs the loopback PKCE flow and stores the remote.
// The refresh token is written to disk and is never returned to the server.
func LoginOIDC(ctx context.Context, req OIDCLoginRequest) (remoteapi.RemoteConfig, error) {
	if req.Issuer == "" || req.ClientID == "" || req.Server == "" {
		return remoteapi.RemoteConfig{}, fmt.Errorf("issuer, client id, and server are required")
	}
	if req.Name == "" {
		req.Name = "team"
	}
	if req.PrincipalClaim == "" {
		req.PrincipalClaim = defaultPrincipalClaim
	}
	if req.DataDir == "" {
		return remoteapi.RemoteConfig{}, fmt.Errorf("data dir is required")
	}
	if req.Timeout <= 0 {
		req.Timeout = 5 * time.Minute
	}
	client := req.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	disc, err := Discover(ctx, client, req.Issuer)
	if err != nil {
		return remoteapi.RemoteConfig{}, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return remoteapi.RemoteConfig{}, err
	}
	redirect := "http://" + ln.Addr().String() + "/callback"
	verifier := oauth2.GenerateVerifier()
	state, err := randomJTI()
	if err != nil {
		ln.Close()
		return remoteapi.RemoteConfig{}, err
	}
	conf := &oauth2.Config{
		ClientID: req.ClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:  disc.AuthorizationEndpoint,
			TokenURL: disc.TokenEndpoint,
			// Public clients have no secret. Basic auth with an empty secret
			// makes some IdPs reject and burn the authorization code.
			AuthStyle: oauth2.AuthStyleInParams,
		},
		RedirectURL: redirect,
		// offline_access is not requested: many IdPs reject it unless the
		// client is explicitly allowed to mint offline tokens. A normal
		// authorization-code refresh token is stored when the IdP returns one.
		Scopes: []string{"openid", "profile"},
	}
	authURL := conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
	code, err := waitForCode(ctx, ln, state, authURL, req)
	if err != nil {
		return remoteapi.RemoteConfig{}, err
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return remoteapi.RemoteConfig{}, fmt.Errorf("token exchange: %w", err)
	}
	access := tok.AccessToken
	if !isJWT(access) {
		return remoteapi.RemoteConfig{}, fmt.Errorf("access token is not a JWT; opaque tokens are not supported")
	}
	principal, err := principalFromAccess(access, req.PrincipalClaim)
	if err != nil {
		return remoteapi.RemoteConfig{}, err
	}
	dir := filepath.Join(req.DataDir, "tokens")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return remoteapi.RemoteConfig{}, err
	}
	tokenPath := filepath.Join(dir, req.Name+".token")
	if err := writeSecret(tokenPath, access); err != nil {
		return remoteapi.RemoteConfig{}, err
	}
	var refreshPath string
	if tok.RefreshToken != "" {
		refreshPath = filepath.Join(dir, req.Name+".refresh")
		if err := writeSecret(refreshPath, tok.RefreshToken); err != nil {
			return remoteapi.RemoteConfig{}, err
		}
	}
	remote := remoteapi.RemoteConfig{
		Name:        req.Name,
		Server:      req.Server,
		Principal:   principal,
		TokenFile:   tokenPath,
		CAFile:      req.CAFile,
		ServerName:  req.ServerName,
		Workspace:   "default",
		Auth:        "oidc",
		Issuer:      strings.TrimRight(req.Issuer, "/"),
		ClientID:    req.ClientID,
		RefreshFile: refreshPath,
	}
	if err := saveRemote(req.DataDir, remote, req.MakeDefault); err != nil {
		return remoteapi.RemoteConfig{}, err
	}
	return remote, nil
}

func waitForCode(ctx context.Context, ln net.Listener, state, authURL string, req OIDCLoginRequest) (string, error) {
	defer ln.Close()
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/callback" {
				http.NotFound(w, r)
				return
			}
			if r.URL.Query().Get("state") != state {
				errCh <- fmt.Errorf("oidc state mismatch")
				http.Error(w, "state mismatch", http.StatusBadRequest)
				return
			}
			if e := r.URL.Query().Get("error"); e != "" {
				errCh <- fmt.Errorf("oidc authorization error: %s", e)
				http.Error(w, "authorization failed", http.StatusBadRequest)
				return
			}
			code := r.URL.Query().Get("code")
			if code == "" {
				errCh <- fmt.Errorf("oidc callback missing code")
				http.Error(w, "missing code", http.StatusBadRequest)
				return
			}
			codeCh <- code
			_, _ = io.WriteString(w, "Login complete. You can close this window.\n")
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()

	fmt.Fprintf(os.Stderr, "Open this URL to log in:\n%s\n", authURL)
	open := req.OpenBrowser
	if open == nil {
		open = OpenBrowser
	}
	if err := open(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "could not open a browser (%v); open the URL above\n", err)
	}

	timer := time.NewTimer(req.Timeout)
	defer timer.Stop()
	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "", fmt.Errorf("timed out waiting for oidc login")
	}
}

// OpenBrowser starts the OS URL handler. Failure is not fatal; the URL is also printed.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// FreshAccessToken returns the stored bearer, refreshing an OIDC access token
// that expires within a minute. The refresh token is not included in the result.
func FreshAccessToken(ctx context.Context, remote remoteapi.RemoteConfig) (string, error) {
	raw, err := os.ReadFile(remote.TokenFile)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	access := strings.TrimSpace(string(raw))
	if remote.Auth != "oidc" {
		return access, nil
	}
	if access != "" && accessStillFresh(access) {
		return access, nil
	}
	if remote.RefreshFile == "" || remote.ClientID == "" || remote.Issuer == "" {
		return "", fmt.Errorf("oidc access token expired; run mnemon auth login --oidc again")
	}
	refreshRaw, err := os.ReadFile(remote.RefreshFile)
	if err != nil {
		return "", fmt.Errorf("read refresh token: %w", err)
	}
	refresh := strings.TrimSpace(string(refreshRaw))
	if refresh == "" {
		return "", fmt.Errorf("oidc access token expired; run mnemon auth login --oidc again")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	disc, err := Discover(ctx, client, remote.Issuer)
	if err != nil {
		return "", err
	}
	conf := &oauth2.Config{
		ClientID: remote.ClientID,
		Endpoint: oauth2.Endpoint{
			TokenURL:  disc.TokenEndpoint,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	src := conf.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh, Expiry: time.Now().Add(-time.Hour)})
	tok, err := src.Token()
	if err != nil {
		return "", fmt.Errorf("oidc refresh failed")
	}
	if !isJWT(tok.AccessToken) {
		return "", fmt.Errorf("refreshed access token is not a JWT")
	}
	if err := writeSecret(remote.TokenFile, tok.AccessToken); err != nil {
		return "", err
	}
	if tok.RefreshToken != "" && tok.RefreshToken != refresh {
		if err := writeSecret(remote.RefreshFile, tok.RefreshToken); err != nil {
			return "", err
		}
	}
	return tok.AccessToken, nil
}

func accessStillFresh(token string) bool {
	exp, err := accessExpiry(token)
	if err != nil {
		return false
	}
	return time.Until(exp) > accessRefreshSkew
}

func accessExpiry(token string) (time.Time, error) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return time.Time{}, err
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return time.Time{}, fmt.Errorf("missing exp")
	}
	return exp.Time, nil
}

func principalFromAccess(token, claim string) (string, error) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return "", fmt.Errorf("access token: %w", err)
	}
	principal, _ := claims[claim].(string)
	principal = strings.TrimSpace(principal)
	if principal == "" {
		return "", fmt.Errorf("access token missing %q", claim)
	}
	return principal, nil
}

func isJWT(token string) bool {
	return strings.Count(token, ".") == 2 && !strings.ContainsAny(token, " \n")
}

func writeSecret(path, value string) error {
	return os.WriteFile(path, []byte(value+"\n"), 0o600)
}

func saveRemote(dataDir string, remote remoteapi.RemoteConfig, makeDefault bool) error {
	path := filepath.Join(dataDir, remoteapi.DefaultAuthFileName)
	cfg, err := LoadAuthConfig(path)
	if err != nil {
		return err
	}
	UpsertRemote(cfg, remote)
	if makeDefault {
		cfg.DefaultRemote = remote.Name
	}
	return SaveAuthConfig(path, cfg)
}
