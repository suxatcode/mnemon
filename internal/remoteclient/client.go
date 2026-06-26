package remoteclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/rpc"
	"os"
	"strings"

	"github.com/mnemon-dev/mnemon/internal/remoteapi"
)

type Client struct {
	rpc  *rpc.Client
	auth remoteapi.Auth
}

func Dial(remote remoteapi.RemoteConfig) (*Client, error) {
	tokenBytes, err := os.ReadFile(remote.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("read token file: %w", err)
	}
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: remote.ServerName,
	}
	if remote.CAFile != "" {
		pem, err := os.ReadFile(remote.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read ca file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("parse ca file: no PEM certificates found")
		}
		cfg.RootCAs = pool
	}

	conn, err := tls.Dial("tcp", remote.Server, cfg)
	if err != nil {
		return nil, fmt.Errorf("dial remote %s: %w", remote.Server, err)
	}
	return &Client{
		rpc: rpc.NewClient(conn),
		auth: remoteapi.Auth{
			Principal: remote.Principal,
			Token:     strings.TrimSpace(string(tokenBytes)),
		},
	}, nil
}

func (c *Client) Close() error {
	return c.rpc.Close()
}

func (c *Client) Auth() remoteapi.Auth {
	return c.auth
}

func (c *Client) call(method string, req any) (*remoteapi.Response, error) {
	var resp remoteapi.Response
	if err := c.rpc.Call(remoteapi.RPCServiceName+"."+method, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Status() (*remoteapi.Response, error) {
	return c.call("Status", remoteapi.StatusRequest{Auth: c.auth})
}

func (c *Client) Remember(req remoteapi.RememberRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Remember", req)
}

func (c *Client) Recall(req remoteapi.RecallRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Recall", req)
}

func (c *Client) Search(req remoteapi.SearchRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Search", req)
}

func (c *Client) Link(req remoteapi.LinkRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Link", req)
}

func (c *Client) Forget(req remoteapi.ForgetRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Forget", req)
}

func (c *Client) Log(req remoteapi.LogRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Log", req)
}

func (c *Client) Related(req remoteapi.RelatedRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Related", req)
}

func (c *Client) GC(req remoteapi.GCRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("GC", req)
}

func (c *Client) Receipt(req remoteapi.ReceiptRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Receipt", req)
}

func (c *Client) Embed(req remoteapi.EmbedRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Embed", req)
}

func (c *Client) Import(req remoteapi.ImportRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Import", req)
}

func (c *Client) Viz(req remoteapi.VizRequest) (*remoteapi.Response, error) {
	req.Auth = c.auth
	return c.call("Viz", req)
}
