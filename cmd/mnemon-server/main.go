package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/suxatcode/mnemon/internal/embed"
	"github.com/suxatcode/mnemon/internal/model"
	"github.com/suxatcode/mnemon/internal/remoteapi"
	"github.com/suxatcode/mnemon/internal/remoteauth"
	"github.com/suxatcode/mnemon/internal/remoteserver"
	"github.com/suxatcode/mnemon/internal/store"
)

var (
	addr        string
	dataDir     string
	storeName   string
	tlsCert     string
	tlsKey      string
	embedModel  string
	databaseURL string
	jwtKeyFile  string
	maxInsights int

	issuePrincipal  string
	issueServer     string
	issueOut        string
	issueCA         string
	issueName       string
	issueRole       string
	issueServerName string
	issueTTLDays    int
	issueTTL        time.Duration

	oidcIssuer         string
	oidcAudience       string
	oidcJWKSURL        string
	oidcPrincipalClaim string
	oidcAllowClaim     string
	oidcAllowValue     string
	oidcRoleClaim      string
	oidcOrgValue       string
	oidcProvision      string
)

func main() {
	root := &cobra.Command{
		Use:   "mnemon-server",
		Short: "Remote Mnemon memory gateway",
	}
	root.AddCommand(serveCmd(), userCmd(), jwtCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func defaultServerDataDir() string {
	if env := os.Getenv("MNEMON_DATA_DIR"); env != "" {
		return env
	}
	return filepath.Join(store.DefaultDataDir(), "server")
}

func serveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the remote Mnemon HTTPS API",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jwtKeyFile == "" && oidcIssuer == "" {
				return fmt.Errorf("--jwt-key is required unless --oidc-issuer is set")
			}
			return remoteserver.Serve(remoteserver.ServeOptions{
				Addr:        addr,
				TLSCert:     tlsCert,
				TLSKey:      tlsKey,
				DataDir:     dataDir,
				StoreName:   storeName,
				DatabaseURL: databaseURL,
				EmbedModel:  embedModel,
				JWTKeyFile:  jwtKeyFile,
				MaxInsights: maxInsights,
				OIDC: remoteauth.OIDCConfig{
					Issuer:         oidcIssuer,
					Audience:       oidcAudience,
					JWKSURL:        oidcJWKSURL,
					PrincipalClaim: oidcPrincipalClaim,
					AllowClaim:     oidcAllowClaim,
					AllowValue:     oidcAllowValue,
					RoleClaim:      oidcRoleClaim,
					OrgValue:       oidcOrgValue,
					Provision:      oidcProvision,
				},
			})
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":7443", "listen address")
	cmd.Flags().StringVar(&dataDir, "data-dir", defaultServerDataDir(), "server data directory (SQLite)")
	cmd.Flags().StringVar(&storeName, "store", store.DefaultStoreName, "server store name (SQLite)")
	cmd.Flags().StringVar(&databaseURL, "database-url", os.Getenv("MNEMON_DATABASE_URL"), "postgres URL; if set, SQLite is not used")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate PEM (required for TLS)")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key PEM")
	cmd.Flags().StringVar(&jwtKeyFile, "jwt-key", "", "HMAC JWT signing key file (min 32 bytes); optional when --oidc-issuer is set")
	cmd.Flags().StringVar(&oidcIssuer, "oidc-issuer", "", "OIDC issuer URL; enables access-token login")
	cmd.Flags().StringVar(&oidcAudience, "oidc-audience", "", "expected access-token audience")
	cmd.Flags().StringVar(&oidcJWKSURL, "oidc-jwks-url", "", "JWKS URL when the issuer URL is not reachable from this process")
	cmd.Flags().StringVar(&oidcPrincipalClaim, "oidc-principal-claim", "preferred_username", "access-token claim used as the principal")
	cmd.Flags().StringVar(&oidcAllowClaim, "oidc-allow-claim", "", "claim that must contain --oidc-allow-value")
	cmd.Flags().StringVar(&oidcAllowValue, "oidc-allow-value", "", "required value of --oidc-allow-claim")
	cmd.Flags().StringVar(&oidcRoleClaim, "oidc-role-claim", "", "claim that grants role=org when it contains --oidc-org-value")
	cmd.Flags().StringVar(&oidcOrgValue, "oidc-org-value", "", "value of --oidc-role-claim that maps to role=org")
	cmd.Flags().StringVar(&oidcProvision, "oidc-provision", remoteauth.ProvisionAuto, "auto (first valid token creates a user) or allowlist")
	cmd.Flags().IntVar(&maxInsights, "max-insights", store.MaxInsightsFromEnv(store.ServerDefaultMaxInsights), "per-principal personal insight cap (env: MNEMON_MAX_INSIGHTS)")
	cmd.Flags().StringVar(&embedModel, "embed-model", "", fmt.Sprintf("Ollama embedding model (default: %s)", embed.DefaultModel))
	return cmd
}

func openIssueDB() (*store.DB, error) {
	return store.OpenWithOptions(store.Options{
		DataDir:     store.StoreDir(dataDir, storeName),
		DatabaseURL: databaseURL,
	})
}

func userCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "user",
		Short: "Manage JWT principals",
	}
	issue := &cobra.Command{
		Use:   "issue",
		Short: "Issue a JWT invite for a user (takes effect immediately, no restart)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if issuePrincipal == "" {
				return fmt.Errorf("--principal is required")
			}
			if issueServer == "" {
				return fmt.Errorf("--server is required")
			}
			if jwtKeyFile == "" {
				return fmt.Errorf("--jwt-key is required")
			}
			if issueRole == "" {
				issueRole = model.RoleUser
			}
			if issueRole != model.RoleUser && issueRole != model.RoleOrg {
				return fmt.Errorf("role must be user or org")
			}
			key, err := remoteauth.LoadKey(jwtKeyFile)
			if err != nil {
				return err
			}
			db, err := openIssueDB()
			if err != nil {
				return err
			}
			defer db.Close()
			fmt.Fprintf(os.Stderr, "mnemon-server: issuing against store %s\n", db.DisplayPath())
			ttl := time.Duration(issueTTLDays) * 24 * time.Hour
			if issueTTL > 0 {
				ttl = issueTTL
			}
			token, ident, err := remoteauth.Issuer{DB: db, Key: key}.Issue(issuePrincipal, issueRole, ttl)
			if err != nil {
				return err
			}
			var caPEM string
			if issueCA != "" {
				data, err := os.ReadFile(issueCA)
				if err != nil {
					return err
				}
				caPEM = string(data)
			}
			invite := remoteapi.Invite{
				SchemaVersion: 1,
				Name:          issueName,
				Server:        issueServer,
				Principal:     ident.Principal,
				Token:         token,
				CAPEM:         caPEM,
				ServerName:    issueServerName,
				Workspace:     "default",
				Role:          ident.Role,
			}
			out, err := json.MarshalIndent(invite, "", "  ")
			if err != nil {
				return err
			}
			out = append(out, '\n')
			if issueOut == "" || issueOut == "-" {
				fmt.Print(string(out))
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(issueOut), 0o755); err != nil {
				return err
			}
			return os.WriteFile(issueOut, out, 0o600)
		},
	}
	issue.Flags().StringVar(&dataDir, "data-dir", defaultServerDataDir(), "server data directory (SQLite)")
	issue.Flags().StringVar(&storeName, "store", store.DefaultStoreName, "server store name (SQLite)")
	issue.Flags().StringVar(&databaseURL, "database-url", os.Getenv("MNEMON_DATABASE_URL"), "postgres URL")
	issue.Flags().StringVar(&jwtKeyFile, "jwt-key", "", "HMAC JWT signing key file")
	issue.Flags().StringVar(&issuePrincipal, "principal", "", "principal to issue")
	issue.Flags().StringVar(&issueRole, "role", model.RoleUser, "role: user or org")
	issue.Flags().IntVar(&issueTTLDays, "ttl-days", 90, "token lifetime in days")
	issue.Flags().DurationVar(&issueTTL, "ttl", 0, "token lifetime (overrides --ttl-days when set)")
	issue.Flags().StringVar(&issueServer, "server", "", "server host:port clients should dial")
	issue.Flags().StringVar(&issueServerName, "server-name", "", "TLS ServerName (SNI), if different from --server")
	issue.Flags().StringVar(&issueOut, "out", "-", "invite file output path")
	issue.Flags().StringVar(&issueCA, "ca-file", "", "CA PEM to embed in the invite")
	issue.Flags().StringVar(&issueName, "name", "team", "suggested remote name")

	revoke := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke all tokens for a principal",
		RunE: func(cmd *cobra.Command, args []string) error {
			if issuePrincipal == "" {
				return fmt.Errorf("--principal is required")
			}
			db, err := openIssueDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.RevokePrincipalTokens(issuePrincipal); err != nil {
				return err
			}
			if err := db.SetPrincipalDisabled(issuePrincipal, true); err != nil {
				return err
			}
			fmt.Printf("revoked principal %s\n", issuePrincipal)
			return nil
		},
	}
	revoke.Flags().StringVar(&dataDir, "data-dir", defaultServerDataDir(), "server data directory (SQLite)")
	revoke.Flags().StringVar(&storeName, "store", store.DefaultStoreName, "server store name")
	revoke.Flags().StringVar(&databaseURL, "database-url", os.Getenv("MNEMON_DATABASE_URL"), "postgres URL")
	revoke.Flags().StringVar(&issuePrincipal, "principal", "", "principal to revoke")

	add := &cobra.Command{
		Use:   "add",
		Short: "Create a principal without issuing a JWT (for OIDC allowlist)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if issuePrincipal == "" {
				return fmt.Errorf("--principal is required")
			}
			if issueRole == "" {
				issueRole = model.RoleUser
			}
			if issueRole != model.RoleUser && issueRole != model.RoleOrg {
				return fmt.Errorf("role must be user or org")
			}
			db, err := openIssueDB()
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.UpsertPrincipal(issuePrincipal, issueRole); err != nil {
				return err
			}
			fmt.Printf("added principal %s role %s\n", issuePrincipal, issueRole)
			return nil
		},
	}
	add.Flags().StringVar(&dataDir, "data-dir", defaultServerDataDir(), "server data directory (SQLite)")
	add.Flags().StringVar(&storeName, "store", store.DefaultStoreName, "server store name")
	add.Flags().StringVar(&databaseURL, "database-url", os.Getenv("MNEMON_DATABASE_URL"), "postgres URL")
	add.Flags().StringVar(&issuePrincipal, "principal", "", "principal to add")
	add.Flags().StringVar(&issueRole, "role", model.RoleUser, "role: user or org")
	root.AddCommand(issue, revoke, add)
	return root
}

func jwtCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "jwt", Short: "JWT key helpers"}
	keygen := &cobra.Command{
		Use:   "keygen",
		Short: "Generate a HMAC JWT signing key",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := remoteauth.GenerateKey()
			if err != nil {
				return err
			}
			if issueOut == "" || issueOut == "-" {
				fmt.Println(key)
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(issueOut), 0o755); err != nil {
				return err
			}
			return os.WriteFile(issueOut, []byte(key+"\n"), 0o600)
		},
	}
	keygen.Flags().StringVar(&issueOut, "out", "-", "output path")
	cmd.AddCommand(keygen)
	return cmd
}
