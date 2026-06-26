package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/remoteauth"
	"github.com/mnemon-dev/mnemon/internal/remoteclient"
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
	if len(resp.JSON) > 0 {
		_, err := os.Stdout.Write(resp.JSON)
		return err
	}
	if resp.Text != "" {
		fmt.Println(resp.Text)
	}
	return nil
}
