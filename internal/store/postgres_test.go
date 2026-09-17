package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mnemon-dev/mnemon/internal/model"
)

func TestMaxInsightsFromEnv(t *testing.T) {
	t.Setenv("MNEMON_MAX_INSIGHTS", "")
	if got := MaxInsightsFromEnv(1000); got != 1000 {
		t.Fatalf("empty env: %d", got)
	}
	t.Setenv("MNEMON_MAX_INSIGHTS", "25000")
	if got := MaxInsightsFromEnv(1000); got != 25000 {
		t.Fatalf("set env: %d", got)
	}
	t.Setenv("MNEMON_MAX_INSIGHTS", "nope")
	if got := MaxInsightsFromEnv(7); got != 7 {
		t.Fatalf("invalid env: %d", got)
	}
}

func TestDialectRebind(t *testing.T) {
	q := rebind(DialectPostgres, `SELECT * FROM insights WHERE id = ? AND owner_principal = ?`)
	if q != `SELECT * FROM insights WHERE id = $1 AND owner_principal = $2` {
		t.Fatalf("rebind: %s", q)
	}
	if rebind(DialectSQLite, `SELECT ?`) != `SELECT ?` {
		t.Fatal("sqlite rebind should be a no-op")
	}
}

func TestPostgresDialectRoundTrip(t *testing.T) {
	db := openPostgresTestDB(t)
	runSharedDialectChecks(t, db)
}

func TestSQLiteDialectRoundTrip(t *testing.T) {
	runSharedDialectChecks(t, testDB(t))
}

func runSharedDialectChecks(t *testing.T, db *DB) {
	t.Helper()
	alice := makeInsight("d-alice", "alice remembers widgets", 3)
	alice.OwnerPrincipal = "alice"
	alice.Layer = model.LayerPersonal
	alice.Entities = []string{"WidgetCo"}
	if err := db.InsertInsight(alice); err != nil {
		t.Fatal(err)
	}
	org := makeInsight("d-org", "org policy uses go 1.24", 5)
	org.OwnerPrincipal = "organization"
	org.Layer = model.LayerOrg
	if err := db.InsertInsight(org); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetInsightByID("d-alice")
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerPrincipal != "alice" || got.Layer != model.LayerPersonal {
		t.Fatalf("owner/layer %+v", got)
	}
	hits, err := db.QueryInsights(QueryFilter{Keyword: "widgets", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].ID != "d-alice" {
		t.Fatalf("keyword query: %+v", hits)
	}
	known, err := db.LoadKnownEntities()
	if err != nil {
		t.Fatal(err)
	}
	if !known["WidgetCo"] {
		t.Fatalf("entities: %v", known)
	}
	personal, err := db.GetActiveInsightsForDiff("alice", model.LayerPersonal)
	if err != nil {
		t.Fatal(err)
	}
	if len(personal) != 1 || personal[0].ID != "d-alice" {
		t.Fatalf("diff owner filter: %+v", personal)
	}
	if err := db.UpsertPrincipal("alice", "user"); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertIssuedToken("jti-1", "alice", time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	tok, err := db.LookupIssuedToken("jti-1")
	if err != nil || tok.Revoked {
		t.Fatalf("token: %v %+v", err, tok)
	}
	pruned, err := db.AutoPruneOwned("alice", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pruned < 1 {
		t.Fatalf("expected alice personal prune, got %d", pruned)
	}
	if _, err := db.GetInsightByID("d-org"); err != nil {
		t.Fatal("org insight must survive personal prune")
	}
}

func openPostgresTestDB(t *testing.T) *DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("TEST_POSTGRES_URL"))
	if base == "" {
		t.Skip("TEST_POSTGRES_URL not set")
	}
	schema := fmt.Sprintf("mnemon_test_%d", time.Now().UnixNano())
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	admin.Close()

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	db, err := OpenWithOptions(Options{DatabaseURL: u.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		admin, err := sql.Open("pgx", base)
		if err == nil {
			_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
			admin.Close()
		}
	})
	return db
}
