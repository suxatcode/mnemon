package deploy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Dir(file)
}

func TestDockerfileServerTarget(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "..", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"FROM alpine:3.22 AS server",
		`ENTRYPOINT ["mnemon-server"]`,
		`CMD ["serve"]`,
		"-o /out/mnemon-server ./cmd/mnemon-server",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Dockerfile missing %q", want)
		}
	}
}

func TestHelmChartSourceHasNoUsersJSON(t *testing.T) {
	root := filepath.Join(repoRoot(t), "helm", "mnemon-server")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if info.Name() == "NOTES.txt" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "users.json") {
			t.Errorf("%s still references users.json", path)
		}
		if strings.Contains(string(data), "--users") {
			t.Errorf("%s still passes --users", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	dep, err := os.ReadFile(filepath.Join(root, "templates", "deployment.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(dep)
	for _, want := range []string{"/health", "/ready", "--jwt-key", "MNEMON_DATABASE_URL", "--max-insights", "projected:", "MNEMON_DATA_DIR"} {
		if !strings.Contains(text, want) {
			t.Errorf("deployment.yaml missing %q", want)
		}
	}
}

func TestHelmTemplateBundledAndExternal(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed")
	}
	chart := filepath.Join(repoRoot(t), "helm", "mnemon-server")

	bundled, err := exec.Command(helm, "template", "mnemon", chart).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template bundled: %v\n%s", err, bundled)
	}
	out := string(bundled)
	if strings.Contains(out, "users.json") {
		t.Fatal("bundled render contains users.json")
	}
	if !strings.Contains(out, "kind: StatefulSet") || !strings.Contains(out, "postgres:16-alpine") {
		t.Fatal("bundled render should include Postgres StatefulSet")
	}
	if !strings.Contains(out, "replicas: 2") {
		t.Fatal("postgres mode should default to 2 replicas")
	}
	if strings.Contains(out, "kind: PersistentVolumeClaim") {
		t.Fatal("bundled postgres should not render a SQLite PVC")
	}
	if !strings.Contains(out, "path: /health") || !strings.Contains(out, "path: /ready") {
		t.Fatal("probes missing")
	}
	if !strings.Contains(out, "jwt.key") {
		t.Fatal("jwt key missing from bundled render")
	}
	if !strings.Contains(out, "app.kubernetes.io/component: server") {
		t.Fatal("server pods/service must set component=server so Postgres is not selected")
	}

	secretSrc, err := os.ReadFile(filepath.Join(chart, "templates", "secret.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	secretText := string(secretSrc)
	for _, want := range []string{`"127.0.0.1"`, `"localhost"`} {
		if !strings.Contains(secretText, want) {
			t.Errorf("TLS cert template missing SAN %s", want)
		}
	}

	external, err := exec.Command(helm, "template", "mnemon", chart,
		"--set", "postgresql.enabled=false",
		"--set", "database.url=postgres://mnemon:secret@rds:5432/mnemon?sslmode=require",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template external: %v\n%s", err, external)
	}
	ext := string(external)
	if strings.Contains(ext, "app.kubernetes.io/component: postgresql") {
		t.Fatal("external DSN must not deploy bundled Postgres")
	}
	if !strings.Contains(ext, "postgres://mnemon:secret@rds:5432/mnemon?sslmode=require") {
		t.Fatal("external DSN missing from render")
	}
	if strings.Contains(ext, "users.json") {
		t.Fatal("external render contains users.json")
	}

	sqlite, err := exec.Command(helm, "template", "mnemon", chart,
		"--set", "postgresql.enabled=false",
		"--set", "persistence.enabled=true",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template sqlite: %v\n%s", err, sqlite)
	}
	sqlOut := string(sqlite)
	if !strings.Contains(sqlOut, "kind: PersistentVolumeClaim") {
		t.Fatal("sqlite mode should include a PVC")
	}
	if !strings.Contains(sqlOut, "replicas: 1") {
		t.Fatal("sqlite mode must be single replica")
	}
	if strings.Contains(sqlOut, "MNEMON_DATABASE_URL") {
		t.Fatal("sqlite mode should not set MNEMON_DATABASE_URL")
	}

	jwtOnly, err := exec.Command(helm, "template", "mnemon", chart,
		"--set", "fullnameOverride=mnemon",
		"--set", "server.existingSecret=mnemon-jwt",
		"--set", "postgresql.enabled=false",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template jwt existingSecret: %v\n%s", err, jwtOnly)
	}
	jwtOut := string(jwtOnly)
	if !strings.Contains(jwtOut, "name: mnemon-jwt") {
		t.Fatal("JWT existingSecret should be mounted")
	}
	if !strings.Contains(jwtOut, "name: mnemon-tls") {
		t.Fatal("JWT existingSecret should still generate a TLS secret")
	}
	if strings.Contains(jwtOut, "name: mnemon-app") {
		t.Fatal("JWT existingSecret should not generate the default app secret")
	}

	urlPlusJWT, err := exec.Command(helm, "template", "mnemon", chart,
		"--set", "fullnameOverride=mnemon",
		"--set", "server.existingSecret=mnemon-jwt",
		"--set", "postgresql.enabled=false",
		"--set", "database.url=postgres://mnemon:secret@pg:5432/mnemon?sslmode=disable",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template jwt+url: %v\n%s", err, urlPlusJWT)
	}
	if !strings.Contains(string(urlPlusJWT), "name: mnemon-database") {
		t.Fatal("database.url with JWT existingSecret should render a DSN secret")
	}

	ing, err := exec.Command(helm, "template", "mnemon", chart,
		"-f", filepath.Join(chart, "values-aws.yaml"),
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template values-aws: %v\n%s", err, ing)
	}
	aws := string(ing)
	for _, want := range []string{
		"kind: Ingress",
		"kind: Certificate",
		"name: mnemon-jwt",
		"secretName: mnemon-tls",
		"name: mnemon-db",
		"mnemon.example.com",
		"cert-manager.io/cluster-issuer",
	} {
		if !strings.Contains(aws, want) {
			t.Errorf("values-aws render missing %q", want)
		}
	}
	if strings.Contains(aws, "kind: StatefulSet") {
		t.Fatal("values-aws must not deploy bundled Postgres")
	}

	notesTpl, err := os.ReadFile(filepath.Join(chart, "templates", "NOTES.txt"))
	if err != nil {
		t.Fatal(err)
	}
	notesSrc := string(notesTpl)
	if !strings.Contains(notesSrc, "HTTPS{{ else }}HTTP") {
		t.Fatal("NOTES.txt should switch HTTPS/HTTP from server.tls.enabled")
	}

	tlsOff, err := exec.Command(helm, "template", "mnemon", chart,
		"--set", "server.tls.enabled=false",
		"--set", "postgresql.enabled=false",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template tls-off: %v\n%s", err, tlsOff)
	}
	off := string(tlsOff)
	if strings.Contains(off, "--tls-cert") {
		t.Fatal("tls-off render must not pass --tls-cert")
	}
}
