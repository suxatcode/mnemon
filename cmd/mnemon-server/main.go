package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/internal/embed"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/remoteauth"
	"github.com/mnemon-dev/mnemon/internal/remoteserver"
	"github.com/mnemon-dev/mnemon/internal/store"
	"github.com/spf13/cobra"
)

var (
	addr       string
	dataDir    string
	storeName  string
	usersFile  string
	tlsCert    string
	tlsKey     string
	embedModel string

	issuePrincipal string
	issueServer    string
	issueOut       string
	issueCA        string
	issueName      string
)

func main() {
	root := &cobra.Command{
		Use:   "mnemon-server",
		Short: "Remote Mnemon memory gateway",
	}
	root.AddCommand(serveCmd(), userCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func defaultServerDataDir() string {
	return filepath.Join(store.DefaultDataDir(), "server")
}

func serveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the remote Mnemon RPC API",
		RunE: func(cmd *cobra.Command, args []string) error {
			if tlsCert == "" || tlsKey == "" {
				return fmt.Errorf("--tls-cert and --tls-key are required")
			}
			return remoteserver.Serve(remoteserver.ServeOptions{
				Addr:       addr,
				TLSCert:    tlsCert,
				TLSKey:     tlsKey,
				UsersFile:  usersFile,
				DataDir:    dataDir,
				StoreName:  storeName,
				EmbedModel: embedModel,
			})
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":7443", "listen address")
	cmd.Flags().StringVar(&dataDir, "data-dir", defaultServerDataDir(), "server data directory")
	cmd.Flags().StringVar(&storeName, "store", store.DefaultStoreName, "server store name")
	cmd.Flags().StringVar(&usersFile, "users", filepath.Join(defaultServerDataDir(), "users.json"), "users file")
	cmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate PEM")
	cmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key PEM")
	cmd.Flags().StringVar(&embedModel, "embed-model", "", fmt.Sprintf("Ollama embedding model (default: %s)", embed.DefaultModel))
	return cmd
}

func userCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "user",
		Short: "Manage token-auth users",
	}
	issue := &cobra.Command{
		Use:   "issue",
		Short: "Issue an invite file for a user",
		RunE: func(cmd *cobra.Command, args []string) error {
			if issuePrincipal == "" {
				return fmt.Errorf("--principal is required")
			}
			if issueServer == "" {
				return fmt.Errorf("--server is required")
			}
			token, err := remoteauth.GenerateToken()
			if err != nil {
				return err
			}
			doc, err := remoteauth.LoadUsers(usersFile)
			if err != nil {
				return err
			}
			remoteauth.UpsertUser(doc, remoteauth.User{
				Principal: issuePrincipal,
				TokenHash: remoteauth.HashToken(token),
				Scopes:    []string{"memory:default"},
			})
			if err := remoteauth.SaveUsers(usersFile, doc); err != nil {
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
				Principal:     issuePrincipal,
				Token:         token,
				CAPEM:         caPEM,
				Workspace:     "default",
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
	issue.Flags().StringVar(&usersFile, "users", filepath.Join(defaultServerDataDir(), "users.json"), "users file to update")
	issue.Flags().StringVar(&issuePrincipal, "principal", "", "principal to issue")
	issue.Flags().StringVar(&issueServer, "server", "", "server host:port clients should dial")
	issue.Flags().StringVar(&issueOut, "out", "-", "invite file output path")
	issue.Flags().StringVar(&issueCA, "ca-file", "", "CA PEM to embed in the invite")
	issue.Flags().StringVar(&issueName, "name", "team", "suggested remote name")
	root.AddCommand(issue)
	return root
}
