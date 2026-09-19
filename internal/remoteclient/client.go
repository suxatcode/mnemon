package remoteclient

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/suxatcode/mnemon/internal/remoteapi"
)

type Client struct {
	http  *http.Client
	base  string
	token string
	agent string
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
	host := remote.Server
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	return &Client{
		http: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: cfg,
			},
		},
		base:  strings.TrimRight(host, "/"),
		token: strings.TrimSpace(string(tokenBytes)),
		agent: "mnemon-cli",
	}, nil
}

func (c *Client) Close() error { return nil }

func (c *Client) call(path string, req any) (*remoteapi.Response, error) {
	var body []byte
	var err error
	if req != nil {
		body, err = json.Marshal(req)
		if err != nil {
			return nil, err
		}
	}
	httpReq, err := http.NewRequest(http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")
	if c.agent != "" {
		httpReq.Header.Set("X-Mnemon-Agent", c.agent)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var env remoteapi.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode response: %w (%s)", err, strings.TrimSpace(string(raw)))
	}
	if env.Error != "" || resp.StatusCode >= 400 {
		if env.Error == "" {
			env.Error = fmt.Sprintf("http %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", env.Error)
	}
	out := &remoteapi.Response{Text: env.Text, Warnings: env.Warnings}
	if env.Result != nil {
		b, err := json.MarshalIndent(env.Result, "", "  ")
		if err != nil {
			return nil, err
		}
		out.JSON = append(b, '\n')
	}
	return out, nil
}

func (c *Client) Status() (*remoteapi.Response, error) { return c.call("/v1/status", struct{}{}) }
func (c *Client) Remember(req remoteapi.RememberRequest) (*remoteapi.Response, error) {
	return c.call("/v1/remember", req)
}
func (c *Client) Recall(req remoteapi.RecallRequest) (*remoteapi.Response, error) {
	return c.call("/v1/recall", req)
}
func (c *Client) Search(req remoteapi.SearchRequest) (*remoteapi.Response, error) {
	return c.call("/v1/search", req)
}
func (c *Client) Link(req remoteapi.LinkRequest) (*remoteapi.Response, error) {
	return c.call("/v1/link", req)
}
func (c *Client) Forget(req remoteapi.ForgetRequest) (*remoteapi.Response, error) {
	return c.call("/v1/forget", req)
}
func (c *Client) Log(req remoteapi.LogRequest) (*remoteapi.Response, error) {
	return c.call("/v1/log", req)
}
func (c *Client) Related(req remoteapi.RelatedRequest) (*remoteapi.Response, error) {
	return c.call("/v1/related", req)
}
func (c *Client) GC(req remoteapi.GCRequest) (*remoteapi.Response, error) {
	return c.call("/v1/gc", req)
}
func (c *Client) Receipt(req remoteapi.ReceiptRequest) (*remoteapi.Response, error) {
	return c.call("/v1/receipt", req)
}
func (c *Client) Embed(req remoteapi.EmbedRequest) (*remoteapi.Response, error) {
	return c.call("/v1/embed", req)
}
func (c *Client) Import(req remoteapi.ImportRequest) (*remoteapi.Response, error) {
	return c.call("/v1/import", req)
}
func (c *Client) Viz(req remoteapi.VizRequest) (*remoteapi.Response, error) {
	return c.call("/v1/viz", req)
}
