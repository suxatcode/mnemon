package kernel

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/mnemon-dev/mnemon/core/contract"
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
		`CREATE TABLE IF NOT EXISTS decisions (decision_id TEXT PRIMARY KEY, op_id TEXT, ingest_seq INTEGER, actor TEXT, status TEXT, payload TEXT NOT NULL);`,
	} {
		if _, err := db.Exec(s); err != nil {
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
	_, err := t.tx.Exec(`INSERT INTO decisions (decision_id,op_id,ingest_seq,actor,status,payload) VALUES (?,?,?,?,?,?)`,
		d.DecisionID, d.OpID, d.IngestSeq, string(d.Actor), string(d.Status), string(b))
	return err
}

// AppendDecision writes a decision in its own txn (used for non-accepted ops — nothing to be atomic with).
func (s *Store) AppendDecision(d contract.Decision) error {
	b, _ := json.Marshal(d)
	_, err := s.db.Exec(`INSERT INTO decisions (decision_id,op_id,ingest_seq,actor,status,payload) VALUES (?,?,?,?,?,?)`,
		d.DecisionID, d.OpID, d.IngestSeq, string(d.Actor), string(d.Status), string(b))
	return err
}
func (s *Store) AppendEvent(ev contract.Event) (int64, error) {
	b, _ := json.Marshal(ev)
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
		_ = json.Unmarshal([]byte(p), &ev)
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
