package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/remoteauth"
	"github.com/mnemon-dev/mnemon/internal/remoteclient"
	"github.com/mnemon-dev/mnemon/internal/store"
)

func authConfigPath() string {
	return filepath.Join(dataDir, remoteapi.DefaultAuthFileName)
}

func tokenDir() string {
	return filepath.Join(dataDir, "tokens")
}

func defaultRemoteConfig() (*remoteapi.RemoteConfig, bool, error) {
	if localOnly {
		return nil, false, nil
	}
	cfg, err := remoteauth.LoadAuthConfig(authConfigPath())
	if err != nil {
		return nil, false, err
	}
	remote, ok := remoteauth.FindDefaultRemote(cfg)
	return remote, ok, nil
}

func defaultRemoteClient() (*remoteclient.Client, bool, error) {
	remote, ok, err := defaultRemoteConfig()
	if err != nil || !ok {
		return nil, ok, err
	}
	client, err := remoteclient.Dial(*remote)
	if err != nil {
		return nil, true, err
	}
	return client, true, nil
}

func printRemoteResponse(resp *remoteapi.Response) error {
	for _, w := range resp.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if len(resp.JSON) > 0 {
		_, err := os.Stdout.Write(resp.JSON)
		return err
	}
	if resp.Text != "" {
		fmt.Print(resp.Text)
	}
	return nil
}

func writeResult(res memorysvc.Result, err error) error {
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if len(res.JSON) > 0 {
		_, err := os.Stdout.Write(res.JSON)
		return err
	}
	if res.Text != "" {
		fmt.Print(res.Text)
	}
	return nil
}

func localActor() memorysvc.Actor {
	p := os.Getenv("MNEMON_PRINCIPAL")
	if p == "" {
		p = model.LocalOwner
	}
	role := os.Getenv("MNEMON_ROLE")
	if role == "" {
		role = memorysvc.RoleUser
	}
	return memorysvc.Actor{Principal: p, Role: role, Agent: "mnemon-cli"}
}

func localService(db *store.DB) *memorysvc.Service {
	return memorysvc.New(db, memorysvc.Options{
		EmbedModel:  resolveEmbedModel(),
		MaxInsights: store.MaxInsightsFromEnv(store.MaxInsights),
		EnforceACL:  false,
		StoreName:   resolveStoreName(),
	})
}

func withLocalService(fn func(*memorysvc.Service, memorysvc.Actor) error) error {
	db, err := openDB()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	return fn(localService(db), localActor())
}
