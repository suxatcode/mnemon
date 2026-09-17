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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/remoteauth"
	"github.com/mnemon-dev/mnemon/internal/remoteclient"
	"github.com/mnemon-dev/mnemon/internal/store"
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

func TestHTTPAuthRememberRecallAndHealth(t *testing.T) {
	work := t.TempDir()
	certPath, keyPath, caPath := writeTestCert(t, work)
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(work, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	key := []byte("0123456789abcdef0123456789abcdef")
	token, _, err := remoteauth.Issuer{DB: db, Key: key}.Issue("alice@example.com", model.RoleUser, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	svc := memorysvc.New(db, memorysvc.Options{EnforceACL: true, MaxInsights: 1000, StoreName: "default"})
	handler := New(svc, remoteauth.StoreVerifier{DB: db, Key: key}, db).Handler()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	srv := &http.Server{Handler: handler}
	go srv.Serve(ln)

	insecure := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	for _, path := range []string{"/health", "/ready"} {
		resp, err := insecure.Get("https://" + ln.Addr().String() + path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Fatalf("%s status %d", path, resp.StatusCode)
		}
		resp.Body.Close()
	}

	tokenFile := filepath.Join(work, "token")
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
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

	client, err := remoteclient.Dial(baseRemote)
	if err != nil {
		t.Fatal(err)
	}
	remember, err := client.Remember(remoteapi.RememberRequest{
		Content:    "shared temporal lobe test memory",
		Category:   "fact",
		Importance: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(remember.JSON), "alice@example.com") {
		t.Fatalf("remember response should include owner: %s", remember.JSON)
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

func TestOwnerIsolationAndOrgLayer(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := memorysvc.New(db, memorysvc.Options{EnforceACL: true, MaxInsights: 100, StoreName: "default"})
	alice := memorysvc.Actor{Principal: "alice", Role: model.RoleUser, Agent: "test"}
	bob := memorysvc.Actor{Principal: "bob", Role: model.RoleUser, Agent: "test"}
	org := memorysvc.Actor{Principal: "organization", Role: model.RoleOrg, Agent: "test"}

	res, err := svc.Remember(alice, memorysvc.RememberInput{Content: "alice private note about widgets", Category: "fact", Importance: 3, NoDiff: true})
	if err != nil {
		t.Fatal(err)
	}
	var aliceOut struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.JSON, &aliceOut); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Forget(bob, memorysvc.ForgetInput{ID: aliceOut.ID}); err == nil {
		t.Fatal("bob should not forget alice memory")
	}
	orgRes, err := svc.Remember(org, memorysvc.RememberInput{Content: "company uses go 1.24 for mnemon", Category: "decision", Importance: 5, NoDiff: true})
	if err != nil {
		t.Fatal(err)
	}
	var orgOut struct {
		ID    string `json:"id"`
		Layer string `json:"layer"`
	}
	if err := json.Unmarshal(orgRes.JSON, &orgOut); err != nil {
		t.Fatal(err)
	}
	if orgOut.Layer != model.LayerOrg {
		t.Fatalf("org layer = %q", orgOut.Layer)
	}
	if _, err := svc.Forget(alice, memorysvc.ForgetInput{ID: orgOut.ID}); err == nil {
		t.Fatal("user should not forget org memory")
	}
	recall, err := svc.Recall(bob, memorysvc.RecallInput{Query: "alice private widgets", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(recall.JSON), "alice private") {
		t.Fatalf("bob should still recall alice memory: %s", recall.JSON)
	}
}

func TestJWTRevoked(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	key := []byte("0123456789abcdef0123456789abcdef")
	token, ident, err := remoteauth.Issuer{DB: db, Key: key}.Issue("carol", model.RoleUser, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	v := remoteauth.StoreVerifier{DB: db, Key: key}
	if _, err := v.Verify(token); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeIssuedToken(ident.JTI); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(token); err == nil {
		t.Fatal("revoked token should fail")
	}
}

func TestReadyFailsWhenDBClosed(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := memorysvc.New(db, memorysvc.Options{EnforceACL: true})
	handler := New(svc, remoteauth.StoreVerifier{DB: db, Key: []byte("0123456789abcdef0123456789abcdef")}, db).Handler()
	db.Close()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready after close: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestForgedPrincipalTagsStripped(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := memorysvc.New(db, memorysvc.Options{EnforceACL: true, MaxInsights: 100})
	res, err := svc.Remember(memorysvc.Actor{Principal: "alice", Role: model.RoleUser, Agent: "cli"}, memorysvc.RememberInput{
		Content: "tag forgery check", Category: "fact", Importance: 3, Tags: "principal:eve,agent:evil", NoDiff: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.JSON), "principal:eve") {
		t.Fatalf("forged principal tag leaked: %s", res.JSON)
	}
	if !strings.Contains(string(res.JSON), "principal:alice") {
		t.Fatalf("missing real principal tag: %s", res.JSON)
	}
}

func TestRememberPrincipalComesFromJWTOnly(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	key := []byte("0123456789abcdef0123456789abcdef")
	token, _, err := remoteauth.Issuer{DB: db, Key: key}.Issue("alice@team", model.RoleUser, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	svc := memorysvc.New(db, memorysvc.Options{EnforceACL: true, MaxInsights: 100})
	handler := New(svc, remoteauth.StoreVerifier{DB: db, Key: key}, db).Handler()
	body := `{"content":"jwt owner check about widgets","category":"fact","importance":3,"no_diff":true,"principal":"eve@evil"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/remember", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "eve@evil") {
		t.Fatalf("forged principal leaked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "alice@team") {
		t.Fatalf("missing jwt sub: %s", rec.Body.String())
	}
}

func TestClientPropagatesWarnings(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(remoteapi.Envelope{
			Result:   map[string]any{"ok": true},
			Warnings: []string{"from-server"},
		})
	}))
	t.Cleanup(srv.Close)
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("unused\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := remoteclient.Dial(remoteapi.RemoteConfig{
		Server:     strings.TrimPrefix(srv.URL, "https://"),
		TokenFile:  tokenFile,
		CAFile:     caPath,
		ServerName: "example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Warnings) != 1 || resp.Warnings[0] != "from-server" {
		t.Fatalf("warnings=%v", resp.Warnings)
	}
}

func TestHealthOK(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	handler := New(memorysvc.New(db, memorysvc.Options{}), remoteauth.StoreVerifier{DB: db, Key: []byte("0123456789abcdef0123456789abcdef")}, db).Handler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("health: %d %s", rec.Code, rec.Body.String())
	}
}
