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
	for _, want := range []string{"/health", "/ready", "--jwt-key", "MNEMON_DATABASE_URL", "--max-insights"} {
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
}
