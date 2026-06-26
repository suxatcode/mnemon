package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/remoteauth"
	"github.com/mnemon-dev/mnemon/internal/remoteclient"
	"github.com/spf13/cobra"
)

var (
	authLoginDefault bool
	authLoginName    string
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage remote Mnemon authentication",
}

var authLoginCmd = &cobra.Command{
	Use:   "login <invite-file>",
	Short: "Install a remote invite file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		var invite remoteapi.Invite
		if err := json.Unmarshal(data, &invite); err != nil {
			return err
		}
		if invite.SchemaVersion != 1 {
			return fmt.Errorf("unsupported invite schema_version %d", invite.SchemaVersion)
		}
		name := authLoginName
		if name == "" {
			name = invite.Name
		}
		if name == "" {
			name = "team"
		}
		if invite.Server == "" || invite.Principal == "" || invite.Token == "" {
			return fmt.Errorf("invite must include server, principal, and token")
		}
		tokenPath := filepath.Join(tokenDir(), name+".token")
		if err := os.MkdirAll(tokenDir(), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(tokenPath, []byte(invite.Token+"\n"), 0o600); err != nil {
			return err
		}
		var caPath string
		if invite.CAPEM != "" {
			caPath = filepath.Join(tokenDir(), name+".ca.pem")
			if err := os.WriteFile(caPath, []byte(invite.CAPEM), 0o600); err != nil {
				return err
			}
		}
		cfg, err := remoteauth.LoadAuthConfig(authConfigPath())
		if err != nil {
			return err
		}
		remoteauth.UpsertRemote(cfg, remoteapi.RemoteConfig{
			Name:       name,
			Server:     invite.Server,
			Principal:  invite.Principal,
			TokenFile:  tokenPath,
			CAFile:     caPath,
			ServerName: invite.ServerName,
			Workspace:  invite.Workspace,
		})
		if authLoginDefault {
			cfg.DefaultRemote = name
		}
		if err := remoteauth.SaveAuthConfig(authConfigPath(), cfg); err != nil {
			return err
		}
		status := "installed"
		if authLoginDefault {
			status = "installed_default"
		}
		out := map[string]any{
			"status":    status,
			"remote":    name,
			"server":    invite.Server,
			"principal": invite.Principal,
			"default":   authLoginDefault,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show remote authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := remoteauth.LoadAuthConfig(authConfigPath())
		if err != nil {
			return err
		}
		out := map[string]any{
			"auth_file":      authConfigPath(),
			"default_remote": cfg.DefaultRemote,
			"remotes":        cfg.Remotes,
		}
		if remote, ok := remoteauth.FindDefaultRemote(cfg); ok {
			client, err := remoteclientStatus(*remote)
			if err != nil {
				out["remote_status"] = "unreachable"
				out["error"] = err.Error()
			} else {
				out["remote_status"] = client
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	},
}

func remoteclientStatus(remote remoteapi.RemoteConfig) (string, error) {
	client, err := defaultRemoteClientFor(remote)
	if err != nil {
		return "", err
	}
	defer client.Close()
	if _, err := client.Status(); err != nil {
		return "", err
	}
	return "ok", nil
}

func defaultRemoteClientFor(remote remoteapi.RemoteConfig) (*remoteclient.Client, error) {
	return remoteclient.Dial(remote)
}

func init() {
	authLoginCmd.Flags().BoolVar(&authLoginDefault, "default", false, "make this remote the default storage backend")
	authLoginCmd.Flags().StringVar(&authLoginName, "name", "", "remote name")
	authCmd.AddCommand(authLoginCmd, authStatusCmd)
	rootCmd.AddCommand(authCmd)
}
