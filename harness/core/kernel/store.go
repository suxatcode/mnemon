package kernel

import (
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }
type Tx struct{ tx *sql.Tx }

func OpenStore(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // kernel is the sole serializer (Invariant #2): one conn => no lock races, no per-conn :memory: split
	for _, s := range []string{
		`CREATE TABLE IF NOT EXISTS resources (kind TEXT, id TEXT, version INTEGER NOT NULL, fields TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(kind,id));`,
		`CREATE TABLE IF NOT EXISTS events (ingest_seq INTEGER PRIMARY KEY AUTOINCREMENT, payload TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS decisions (decision_id TEXT PRIMARY KEY, op_id TEXT, ingest_seq INTEGER, actor TEXT, correlation_id TEXT, next_action TEXT, status TEXT, payload TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS cursors (name TEXT PRIMARY KEY, seq INTEGER NOT NULL);`,
	} {
		if _, err := db.Exec(s); err != nil {
			db.Close()
			return nil, err
		}
	}
	// Additive migrations for columns introduced after the initial schema, so OpenStore tolerates a
	// decisions table created by older code (CREATE TABLE IF NOT EXISTS is a no-op on an existing table).
	// ALTER ... ADD COLUMN is idempotent here: a "duplicate column" error (fresh DB already has it) is ignored.
	for _, col := range []string{"correlation_id TEXT", "next_action TEXT"} {
		if _, err := db.Exec(`ALTER TABLE decisions ADD COLUMN ` + col); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) WithTx(fn func(*Tx) error) error { // the atomic boundary: check+write are one op (Invariant #3,#5)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(&Tx{tx: tx}); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
func (s *Store) GetVersion(ref contract.ResourceRef) (contract.Version, error) {
	var v contract.Version
	err := s.db.QueryRow(`SELECT version FROM resources WHERE kind=? AND id=?`, string(ref.Kind), string(ref.ID)).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
func (t *Tx) CreateResource(ref contract.ResourceRef, fields map[string]any) error {
	b, err := json.Marshal(fields)
	if err != nil {
		return err
	} // propagate, never write "" silently
	_, err = t.tx.Exec(`INSERT INTO resources (kind,id,version,fields,updated_at) VALUES (?,?,1,?,?)`,
		string(ref.Kind), string(ref.ID), string(b), time.Now().UTC().Format(time.RFC3339))
	return err // PK violation => already exists => caller treats as conflict
}
func (t *Tx) CASUpdate(ref contract.ResourceRef, basedOn contract.Version, fields map[string]any) (bool, error) {
	b, err := json.Marshal(fields)
	if err != nil {
		return false, err
	}
	res, err := t.tx.Exec(`UPDATE resources SET fields=?, version=version+1, updated_at=? WHERE kind=? AND id=? AND version=?`,
		string(b), time.Now().UTC().Format(time.RFC3339), string(ref.Kind), string(ref.ID), int64(basedOn))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
func (t *Tx) ReadVersion(ref contract.ResourceRef) (contract.Version, error) {
	var v contract.Version
	err := t.tx.QueryRow(`SELECT version FROM resources WHERE kind=? AND id=?`, string(ref.Kind), string(ref.ID)).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}

// AppendDecisionTx writes a decision INSIDE a caller's txn (used for accepted ops — crash-safe atomicity, Invariant #7).
func (t *Tx) AppendDecisionTx(d contract.Decision) error {
	b, _ := json.Marshal(d)
	_, err := t.tx.Exec(`INSERT INTO decisions (decision_id,op_id,ingest_seq,actor,correlation_id,next_action,status,payload) VALUES (?,?,?,?,?,?,?,?)`,
		d.DecisionID, d.OpID, d.IngestSeq, string(d.Actor), d.CorrelationID, d.NextAction, string(d.Status), string(b))
	return err
}

// Tx-scoped variants for the atomic dispatch transaction.
func (t *Tx) AppendEvent(ev contract.Event) error {
	b, err := json.Marshal(ev) // never write a garbage payload silently (mirrors Store.AppendEvent)
	if err != nil {
		return err
	}
	_, err = t.tx.Exec(`INSERT INTO events (payload) VALUES (?)`, string(b))
	return err
}
func (t *Tx) SetCursor(name string, seq int64) error {
	_, err := t.tx.Exec(`INSERT INTO cursors (name,seq) VALUES (?,?) ON CONFLICT(name) DO UPDATE SET seq=excluded.seq`, name, seq)
	return err
}

// AppendDecision writes a decision in its own txn (used for non-accepted ops — nothing to be atomic with).
func (s *Store) AppendDecision(d contract.Decision) error {
	b, _ := json.Marshal(d)
	_, err := s.db.Exec(`INSERT INTO decisions (decision_id,op_id,ingest_seq,actor,correlation_id,next_action,status,payload) VALUES (?,?,?,?,?,?,?,?)`,
		d.DecisionID, d.OpID, d.IngestSeq, string(d.Actor), d.CorrelationID, d.NextAction, string(d.Status), string(b))
	return err
}
func (s *Store) AppendEvent(ev contract.Event) (int64, error) {
	b, err := json.Marshal(ev) // this is the durable ingest stream — never write a garbage payload silently
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO events (payload) VALUES (?)`, string(b))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId() // = IngestSeq (Invariant #9)
}
func (s *Store) PendingEvents(afterSeq int64) ([]contract.Event, error) {
	rows, err := s.db.Query(`SELECT ingest_seq, payload FROM events WHERE ingest_seq>? ORDER BY ingest_seq`, afterSeq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.Event
	for rows.Next() {
		var seq int64
		var p string
		if err := rows.Scan(&seq, &p); err != nil {
			return nil, err
		}
		var ev contract.Event
		if err := json.Unmarshal([]byte(p), &ev); err != nil {
			return nil, err // surface a corrupt payload; never manufacture a zero-value event
		}
		ev.IngestSeq = seq
		out = append(out, ev)
	}
	return out, rows.Err()
}
func (s *Store) DecisionCount() int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM decisions`).Scan(&n)
	return n
}

// MaxDecidedSeq returns the highest event ingest_seq that already has a decision (0 if none, or if only
// direct non-event ops have been applied — those carry ingest_seq 0). The decision log IS the
// reconciler's durable cursor: a decision row means that event was consumed, so a fresh Reconciler
// resumes from here instead of re-reading from 0 (Invariant #9 — exactly-once over a contiguous prefix).
func (s *Store) MaxDecidedSeq() int64 {
	var n int64
	_ = s.db.QueryRow(`SELECT COALESCE(MAX(ingest_seq), 0) FROM decisions`).Scan(&n)
	return n
}

// GetCursor returns a named durable cursor (0 if unset). The runtime's dispatch position lives here, the
// way the reconciler's decision position is derived from MaxDecidedSeq — both make the loop restart-safe.
func (s *Store) GetCursor(name string) int64 {
	var seq int64
	err := s.db.QueryRow(`SELECT seq FROM cursors WHERE name=?`, name).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0
	}
	return seq
}
func (s *Store) SetCursor(name string, seq int64) error {
	_, err := s.db.Exec(`INSERT INTO cursors (name,seq) VALUES (?,?) ON CONFLICT(name) DO UPDATE SET seq=excluded.seq`, name, seq)
	return err
}

// DispatchTx atomically appends all proposed events produced from ONE observed event AND advances the
// dispatch cursor past it. All-or-nothing (Invariant R6 / finding #1): a crash can never leave appended
// proposals with an un-advanced cursor (which would re-dispatch and duplicate).
func (s *Store) DispatchTx(events []contract.Event, cursorName string, seq int64) error {
	return s.WithTx(func(tx *Tx) error {
		for _, ev := range events {
			if err := tx.AppendEvent(ev); err != nil {
				return err
			}
		}
		return tx.SetCursor(cursorName, seq)
	})
}

// DeferralCount returns how many REBASE deferrals a CorrelationID has accumulated in the durable log.
// It is the liveness-escalation counter (Invariant #10) derived from the decision log rather than held
// in memory, so escalation survives a process restart exactly as the cursor does. It counts ONLY
// next_action='rebase' deferrals, so an unrelated human_review deferral (from a defer_to_human /
// auto_merge_disjoint pass that shares the CorrelationID) does NOT pre-seed the count and trigger
// premature escalation.
//
// Scope note: for the reconciler path — the SOLE production caller of Kernel.Apply — this reproduces the
// removed in-memory map's predicate exactly (rebase-deferrals are only produced inside RunOnce). A direct
// (non-reconciler) Apply sharing a CorrelationID would also contribute to this durable count; that is not
// reachable today and is the intended behaviour for any future direct path (e.g. CLI-inline, Invariant #17).
func (s *Store) DeferralCount(correlationID string) int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM decisions WHERE correlation_id=? AND status='deferred' AND next_action='rebase'`, correlationID).Scan(&n)
	return n
}

// DecisionsForActor returns this actor's deferred decisions (the pull-feedback source, Invariant #8).
func (s *Store) DecisionsForActor(actor contract.ActorID) ([]contract.Decision, error) {
	rows, err := s.db.Query(`SELECT payload FROM decisions WHERE actor=? AND status='deferred' ORDER BY ingest_seq, rowid`, string(actor))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.Decision
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		var d contract.Decision
		_ = json.Unmarshal([]byte(p), &d)
		out = append(out, d)
	}
	return out, rows.Err()
}
