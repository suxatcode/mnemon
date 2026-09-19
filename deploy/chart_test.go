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

func helmBin(t *testing.T) string {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm not installed")
	}
	return helm
}

func chartDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "helm", "mnemon-server")
}

func helmTemplate(t *testing.T, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"template", "mnemon", chartDir(t)}, args...)
	out, err := exec.Command(helmBin(t), cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template %v: %v\n%s", args, err, out)
	}
	return string(out)
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
		"USER 65532:65532",
		"adduser -S -u 65532",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Dockerfile missing %q", want)
		}
	}
}

func TestHelmChartSourceHasNoUsersJSON(t *testing.T) {
	root := chartDir(t)
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
		text := string(data)
		if strings.Contains(text, "users.json") {
			t.Errorf("%s still references users.json", path)
		}
		if strings.Contains(text, "--users") {
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

func TestHelmChartMetadata(t *testing.T) {
	chart, err := os.ReadFile(filepath.Join(chartDir(t), "Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(chart)
	for _, want := range []string{
		"name: mnemon-server",
		"version: 0.1.1",
		"home: https://github.com/suxatcode/mnemon",
		"org.opencontainers.image.source: https://github.com/suxatcode/mnemon",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("Chart.yaml missing %q", want)
		}
	}
	values, err := os.ReadFile(filepath.Join(chartDir(t), "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text = string(values)
	if !strings.Contains(text, "repository: ghcr.io/suxatcode/mnemon-server") {
		t.Fatal("values.yaml must default image.repository to the public GHCR image")
	}
	if !strings.Contains(text, "fullnameOverride: mnemon") {
		t.Fatal("values.yaml must default fullnameOverride to mnemon")
	}
}

func TestHelmPackage(t *testing.T) {
	dir := t.TempDir()
	out, err := exec.Command(helmBin(t), "package", chartDir(t), "--destination", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("helm package: %v\n%s", err, out)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "mnemon-server-*.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one packaged chart, got %v", matches)
	}
}

func TestHelmChartDoesNotBundlePostgres(t *testing.T) {
	root := chartDir(t)
	for _, name := range []string{"postgres.yaml", "postgres-secret.yaml"} {
		path := filepath.Join(root, "templates", name)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("bundled postgres template must not exist: %s", name)
		}
	}
	values, err := os.ReadFile(filepath.Join(root, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(values), "postgresql:") {
		t.Fatal("values.yaml must not define a bundled postgresql block")
	}
}

func TestHelmTemplateModes(t *testing.T) {
	helmBin(t)

	out := helmTemplate(t)
	if !strings.Contains(out, "image: \"ghcr.io/suxatcode/mnemon-server:dev\"") {
		t.Fatal("default render must pull the public GHCR image")
	}
	if strings.Contains(out, "mnemon-mnemon-server") {
		t.Fatal("default fullname must be mnemon, not mnemon-mnemon-server")
	}
	if !strings.Contains(out, "name: mnemon-app") {
		t.Fatal("default JWT secret should be mnemon-app")
	}
	if strings.Contains(out, "users.json") {
		t.Fatal("default render contains users.json")
	}
	if strings.Contains(out, "kind: StatefulSet") || strings.Contains(out, "postgres:16-alpine") {
		t.Fatal("default render must not include a Postgres StatefulSet")
	}
	if strings.Contains(out, "kind: PersistentVolumeClaim") {
		t.Fatal("default sqlite emptyDir should not render a PVC")
	}
	if !strings.Contains(out, "replicas: 1") {
		t.Fatal("default sqlite mode must be single replica")
	}
	if strings.Contains(out, "MNEMON_DATABASE_URL") {
		t.Fatal("default sqlite mode should not set MNEMON_DATABASE_URL")
	}
	if !strings.Contains(out, "path: /health") || !strings.Contains(out, "path: /ready") {
		t.Fatal("probes missing")
	}
	if !strings.Contains(out, "jwt.key") {
		t.Fatal("jwt key missing from default render")
	}
	if !strings.Contains(out, "app.kubernetes.io/component: server") {
		t.Fatal("server pods/service must set component=server")
	}
	if !strings.Contains(out, "name: https") {
		t.Fatal("default in-pod TLS should name the Service port https")
	}
	if !strings.Contains(out, "kind: ServiceAccount") {
		t.Fatal("default render should include a ServiceAccount")
	}
	if !strings.Contains(out, "runAsUser: 65532") {
		t.Fatal("default render should run as UID 65532")
	}
	if strings.Contains(out, "kind: PodDisruptionBudget") {
		t.Fatal("sqlite single replica should not emit a PDB")
	}
	if strings.Contains(out, "kind: HorizontalPodAutoscaler") {
		t.Fatal("default render should not enable HPA")
	}

	secretSrc, err := os.ReadFile(filepath.Join(chartDir(t), "templates", "secret.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	secretText := string(secretSrc)
	for _, want := range []string{`"127.0.0.1"`, `"localhost"`} {
		if !strings.Contains(secretText, want) {
			t.Errorf("TLS cert template missing SAN %s", want)
		}
	}

	ext := helmTemplate(t,
		"--set", "replicaCount=2",
		"--set", "database.url=postgres://mnemon:secret@rds:5432/mnemon?sslmode=require",
	)
	if strings.Contains(ext, "kind: StatefulSet") || strings.Contains(ext, "app.kubernetes.io/component: postgresql") {
		t.Fatal("external DSN must not deploy bundled Postgres")
	}
	if !strings.Contains(ext, "postgres://mnemon:secret@rds:5432/mnemon?sslmode=require") {
		t.Fatal("external DSN missing from render")
	}
	if !strings.Contains(ext, "replicas: 2") {
		t.Fatal("postgres mode should honor replicaCount")
	}
	if strings.Contains(ext, "kind: PersistentVolumeClaim") {
		t.Fatal("postgres mode should not render a SQLite PVC")
	}
	if !strings.Contains(ext, "kind: PodDisruptionBudget") {
		t.Fatal("postgres replicaCount=2 should emit a PDB")
	}

	sqlOut := helmTemplate(t, "--set", "persistence.enabled=true")
	if !strings.Contains(sqlOut, "kind: PersistentVolumeClaim") {
		t.Fatal("sqlite mode should include a PVC when persistence.enabled")
	}
	if !strings.Contains(sqlOut, "replicas: 1") {
		t.Fatal("sqlite mode must be single replica")
	}
	if strings.Contains(sqlOut, "MNEMON_DATABASE_URL") {
		t.Fatal("sqlite mode should not set MNEMON_DATABASE_URL")
	}

	jwtOut := helmTemplate(t,
		"--set", "fullnameOverride=mnemon",
		"--set", "server.existingSecret=mnemon-jwt",
	)
	if !strings.Contains(jwtOut, "name: mnemon-jwt") {
		t.Fatal("JWT existingSecret should be mounted")
	}
	if !strings.Contains(jwtOut, "name: mnemon-tls") {
		t.Fatal("JWT existingSecret should still generate a TLS secret")
	}
	if strings.Contains(jwtOut, "name: mnemon-app") {
		t.Fatal("JWT existingSecret should not generate the default app secret")
	}

	urlPlusJWT := helmTemplate(t,
		"--set", "fullnameOverride=mnemon",
		"--set", "server.existingSecret=mnemon-jwt",
		"--set", "database.url=postgres://mnemon:secret@pg:5432/mnemon?sslmode=disable",
	)
	if !strings.Contains(urlPlusJWT, "name: mnemon-database") {
		t.Fatal("database.url with JWT existingSecret should render a DSN secret")
	}

	aws := helmTemplate(t, "-f", filepath.Join(chartDir(t), "values-aws.yaml"))
	for _, want := range []string{
		"kind: Ingress",
		"kind: Certificate",
		"name: mnemon-jwt",
		"secretName: mnemon-tls",
		"name: mnemon-db",
		"mnemon.example.com",
		"cert-manager.io/cluster-issuer",
		"ingressClassName: \"nginx\"",
		"group: \"cert-manager.io\"",
		"name: http",
	} {
		if !strings.Contains(aws, want) {
			t.Errorf("values-aws render missing %q", want)
		}
	}
	if strings.Contains(aws, "kind: StatefulSet") {
		t.Fatal("values-aws must not deploy bundled Postgres")
	}
	if strings.Contains(aws, "--tls-cert") {
		t.Fatal("values-aws edge TLS must not pass --tls-cert to the pod")
	}

	istio := helmTemplate(t, "-f", filepath.Join(chartDir(t), "values-istio.yaml"))
	for _, want := range []string{
		"name: http",
		"containerPort: 8080",
		"name: mnemon-db",
		"--addr=:8080",
	} {
		if !strings.Contains(istio, want) {
			t.Errorf("values-istio render missing %q", want)
		}
	}
	if strings.Contains(istio, "kind: Ingress") {
		t.Fatal("values-istio must not create an Ingress; the mesh Gateway is external")
	}
	if strings.Contains(istio, "kind: Certificate") {
		t.Fatal("values-istio must not create a cert-manager Certificate")
	}
	if strings.Contains(istio, "--tls-cert") {
		t.Fatal("values-istio must not pass --tls-cert")
	}
	if strings.Contains(istio, "kind: StatefulSet") {
		t.Fatal("values-istio must not deploy bundled Postgres")
	}

	hostClass := helmTemplate(t,
		"--set", "hostname=mem.team.example",
		"--set", "ingress.enabled=true",
		"--set", "ingress.className=alb",
		"--set", "certManager.enabled=true",
		"--set", "certManager.issuerName=letsencrypt-prod",
		"--set", "certManager.secretName=mem-tls",
		"--set", "server.tls.enabled=false",
	)
	for _, want := range []string{
		"host: \"mem.team.example\"",
		"ingressClassName: \"alb\"",
		"- \"mem.team.example\"",
		"secretName: mem-tls",
		"name: letsencrypt-prod",
	} {
		if !strings.Contains(hostClass, want) {
			t.Errorf("hostname/class/cert-manager render missing %q", want)
		}
	}

	hpa := helmTemplate(t,
		"--set", "autoscaling.enabled=true",
		"--set", "replicaCount=2",
		"--set", "database.url=postgres://mnemon:secret@pg:5432/mnemon?sslmode=disable",
	)
	if !strings.Contains(hpa, "kind: HorizontalPodAutoscaler") {
		t.Fatal("autoscaling.enabled with Postgres should emit an HPA")
	}
	if strings.Contains(hpa, "\n  replicas:") {
		t.Fatal("HPA mode must not set Deployment replicas")
	}

	notesTpl, err := os.ReadFile(filepath.Join(chartDir(t), "templates", "NOTES.txt"))
	if err != nil {
		t.Fatal(err)
	}
	notesSrc := string(notesTpl)
	if !strings.Contains(notesSrc, "HTTPS{{ else }}HTTP") {
		t.Fatal("NOTES.txt should switch HTTPS/HTTP from server.tls.enabled")
	}
	if !strings.Contains(notesSrc, "Postgres is always external") {
		t.Fatal("NOTES.txt should say Postgres is always external")
	}
	if !strings.Contains(notesSrc, "user revoke") {
		t.Fatal("NOTES.txt should document principal revoke")
	}

	off := helmTemplate(t, "--set", "server.tls.enabled=false")
	if strings.Contains(off, "--tls-cert") {
		t.Fatal("tls-off render must not pass --tls-cert")
	}
	if !strings.Contains(off, "name: http") {
		t.Fatal("tls-off Service port must be named http")
	}
}
