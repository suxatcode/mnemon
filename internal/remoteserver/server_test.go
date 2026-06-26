package remoteserver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/remoteauth"
	"github.com/mnemon-dev/mnemon/internal/remoteclient"
	"github.com/mnemon-dev/mnemon/internal/remotesvc"
)

func writeTestCert(t *testing.T, dir string) (certPath, keyPath, caPath string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPath = filepath.Join(dir, "tls.crt")
	keyPath = filepath.Join(dir, "tls.key")
	caPath = filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath, caPath
}

func TestRPCAuthAndRememberRecall(t *testing.T) {
	work := t.TempDir()
	certPath, keyPath, caPath := writeTestCert(t, work)
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	users := &remoteauth.UsersFile{SchemaVersion: 1, Users: []remoteauth.User{
		{Principal: "alice@example.com", TokenHash: remoteauth.HashToken("good-token")},
	}}
	server := rpc.NewServer()
	if err := server.RegisterName(remoteapi.RPCServiceName, New(remotesvc.Service{DataDir: filepath.Join(work, "data")}, users, "")); err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go server.ServeConn(conn)
		}
	}()

	tokenFile := filepath.Join(work, "token")
	if err := os.WriteFile(tokenFile, []byte("good-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	badTokenFile := filepath.Join(work, "bad-token")
	if err := os.WriteFile(badTokenFile, []byte("bad-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	baseRemote := remoteapi.RemoteConfig{
		Server:     ln.Addr().String(),
		Principal:  "alice@example.com",
		TokenFile:  tokenFile,
		CAFile:     caPath,
		ServerName: "localhost",
	}
	badRemote := baseRemote
	badRemote.TokenFile = badTokenFile
	badClient, err := remoteclient.Dial(badRemote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := badClient.Status(); err == nil {
		t.Fatal("bad token should be rejected")
	}
	_ = badClient.Close()

	client, err := remoteclient.Dial(baseRemote)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	remember, err := client.Remember(remoteapi.RememberRequest{
		Content:    "shared temporal lobe test memory",
		Category:   "fact",
		Importance: 3,
		Agent:      "test-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(remember.JSON), "principal:alice@example.com") {
		t.Fatalf("remember response should include provenance tag: %s", remember.JSON)
	}
	recall, err := client.Recall(remoteapi.RecallRequest{Query: "temporal lobe", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Results []struct {
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(recall.JSON, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Results) == 0 || !strings.Contains(payload.Results[0].Content, "shared temporal lobe") {
		t.Fatalf("recall did not return remembered content: %s", recall.JSON)
	}
}
