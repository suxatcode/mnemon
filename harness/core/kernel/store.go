package kernel

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	release func() error // single-writer lock release; no-op for :memory: (S11)
}
type Tx struct{ tx *sql.Tx }

// dsnFor builds the connection DSN. File-backed stores pin synchronous(FULL) (durability: every commit
// fsyncs before returning, closing the WAL crash-window) on top of busy_timeout + WAL. :memory: stays bare.
func dsnFor(path string) string {
	if path == ":memory:" {
		return path
	}
	return path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
}

func OpenStore(path string) (*Store, error) {
	// S11: single-writer lock + anti-NFS guard (skipped for :memory:). Acquire BEFORE opening the DB so a
	// second writer is rejected before it can touch the WAL.
	release, err := openGuard(path, defaultStatFS)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsnFor(path))
	if err != nil {
		_ = release()
		return nil, err
	}
	db.SetMaxOpenConns(1) // kernel is the sole serializer (Invariant #2): one conn => no lock races, no per-conn :memory: split
	for _, s := range []string{
		`CREATE TABLE IF NOT EXISTS resources (kind TEXT, id TEXT, version INTEGER NOT NULL, fields TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(kind,id));`,
		`CREATE TABLE IF NOT EXISTS events (ingest_seq INTEGER PRIMARY KEY AUTOINCREMENT, payload TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS decisions (decision_id TEXT PRIMARY KEY, op_id TEXT, ingest_seq INTEGER, actor TEXT, correlation_id TEXT, next_action TEXT, status TEXT, payload TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS cursors (name TEXT PRIMARY KEY, seq INTEGER NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS inbox_dedupe (source TEXT, external_id TEXT, event_seq INTEGER NOT NULL, PRIMARY KEY(source,external_id));`,
		`CREATE TABLE IF NOT EXISTS outbox (id TEXT PRIMARY KEY, kind TEXT NOT NULL, event_seq INTEGER NOT NULL DEFAULT 0, target TEXT NOT NULL DEFAULT '', payload TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending', idempotency_key TEXT UNIQUE, attempts INTEGER NOT NULL DEFAULT 0, lease_owner TEXT NOT NULL DEFAULT '', lease_until INTEGER NOT NULL DEFAULT 0);`,
	} {
		if _, err := db.Exec(s); err != nil {
			db.Close()
			_ = release()
			return nil, err
		}
	}
	// Additive migrations for columns introduced after the initial schema, so OpenStore tolerates a
	// decisions table created by older code (CREATE TABLE IF NOT EXISTS is a no-op on an existing table).
	// ALTER ... ADD COLUMN is idempotent here: a "duplicate column" error (fresh DB already has it) is ignored.
	for _, col := range []string{"correlation_id TEXT", "next_action TEXT"} {
		if _, err := db.Exec(`ALTER TABLE decisions ADD COLUMN ` + col); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			db.Close()
			_ = release()
			return nil, err
		}
	}
	return &Store{db: db, release: release}, nil
}
func (s *Store) Close() error {
	err := s.db.Close()
	if s.release != nil {
		if rerr := s.release(); err == nil {
			err = rerr
		}
	}
	return err
}

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

// GetResource returns a resource's current version AND decoded field content (review #5). The content digest
// (D8/S10), budget reserve (S6), and lease TTL read all need the fields, not just the version. Absent ->
// (0, nil, nil), consistent with GetVersion.
func (s *Store) GetResource(ref contract.ResourceRef) (contract.Version, map[string]any, error) {
	return scanResource(s.db.QueryRow(`SELECT version, fields FROM resources WHERE kind=? AND id=?`, string(ref.Kind), string(ref.ID)))
}

// ReadResource is GetResource inside a caller's txn — required by the read-modify-write lease/budget claims
// (S5/S6), which read the current version+fields and CAS in the same transaction.
func (t *Tx) ReadResource(ref contract.ResourceRef) (contract.Version, map[string]any, error) {
	return scanResource(t.tx.QueryRow(`SELECT version, fields FROM resources WHERE kind=? AND id=?`, string(ref.Kind), string(ref.ID)))
}

// scanResource decodes a (version, fields) row, mapping ErrNoRows to the absent (0, nil, nil) form. The
// rowScanner seam lets both the Store and Tx variants share decode + JSON-unmarshal logic.
func scanResource(row rowScanner) (contract.Version, map[string]any, error) {
	var v contract.Version
	var b string
	if err := row.Scan(&v, &b); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil, nil
		}
		return 0, nil, err
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(b), &fields); err != nil {
		return 0, nil, err
	}
	return v, fields, nil
}

type rowScanner interface{ Scan(dest ...any) error }

// AppendEventReturningSeq appends an event INSIDE a caller's txn and returns its durable LSN (events.rowid).
// IngestObservation (S1) needs append + LSN-read in one transaction so the dedupe row records the same seq.
func (t *Tx) AppendEventReturningSeq(ev contract.Event) (int64, error) {
	b, err := json.Marshal(ev) // never write a garbage payload silently (mirrors Store.AppendEvent)
	if err != nil {
		return 0, err
	}
	res, err := t.tx.Exec(`INSERT INTO events (payload) VALUES (?)`, string(b))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// IngestObservation appends an observation exactly once per (Source,ExternalID): a retried envelope returns
// the original seq with dup=true and never double-appends (S1). The dedupe lookup, the event append (with its
// LSN via AppendEventReturningSeq, review #6), and the dedupe-row insert are ONE transaction; the single
// writer connection serializes concurrent ingests, so the SELECT-then-INSERT cannot race. The server stamps
// env.Event.Actor from the authenticated principal BEFORE calling (D7) — the store trusts the envelope.
func (s *Store) IngestObservation(env contract.ObservationEnvelope) (int64, bool, error) {
	// An idempotency key is REQUIRED for exactly-once ingest (S1): an empty ExternalID would make every
	// keyless observation from a source collapse onto one dedupe row, silently dropping all but the first.
	if env.ExternalID == "" {
		return 0, false, fmt.Errorf("ingest: empty external_id (an idempotency key is required for exactly-once ingest, S1)")
	}
	var seq int64
	var dup bool
	err := s.WithTx(func(tx *Tx) error {
		existing, found, e := tx.lookupDedupe(env.Source, env.ExternalID)
		if e != nil {
			return e
		}
		if found {
			seq, dup = existing, true
			return nil
		}
		s2, e := tx.AppendEventReturningSeq(env.Event)
		if e != nil {
			return e
		}
		if e := tx.insertDedupe(env.Source, env.ExternalID, s2); e != nil {
			return e
		}
		seq = s2
		return nil
	})
	return seq, dup, err
}

func (t *Tx) lookupDedupe(source contract.ActorID, externalID string) (int64, bool, error) {
	var seq int64
	err := t.tx.QueryRow(`SELECT event_seq FROM inbox_dedupe WHERE source=? AND external_id=?`, string(source), externalID).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return seq, true, nil
}
func (t *Tx) insertDedupe(source contract.ActorID, externalID string, seq int64) error {
	_, err := t.tx.Exec(`INSERT INTO inbox_dedupe (source, external_id, event_seq) VALUES (?,?,?)`, string(source), externalID, seq)
	return err
}

// OutboxRow is one pending external effect (a projection invalidation, a job to run). The outbox is the
// transactional-outbox substrate (S2: enqueued in the SAME tx as the decision that produced it; S4: delivery
// is at-least-once with a per-row lease + an idempotency key, NEVER exactly-once).
type OutboxRow struct {
	ID             string
	Kind           string
	EventSeq       int64
	Target         string
	Payload        string
	Status         string
	IdempotencyKey string
	Attempts       int
	LeaseOwner     string
	LeaseUntil     int64 // unix seconds; 0 = unleased
}

// EnqueueOutbox inserts a pending effect INSIDE a caller's txn so it commits atomically with the decision
// (S2). A duplicate idempotency key is a silent no-op (S4 — at-least-once delivery must never enqueue the
// same effect twice). An empty key is stored as NULL: multiple NULLs are distinct in a UNIQUE index, so
// keyless rows (e.g. invalidations) never collide with each other.
func (t *Tx) EnqueueOutbox(row OutboxRow) error {
	var key any
	if row.IdempotencyKey != "" {
		key = row.IdempotencyKey
	}
	status := row.Status
	if status == "" {
		status = "pending"
	}
	_, err := t.tx.Exec(
		`INSERT INTO outbox (id,kind,event_seq,target,payload,status,idempotency_key,attempts,lease_owner,lease_until)
		 VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(idempotency_key) DO NOTHING`,
		row.ID, row.Kind, row.EventSeq, row.Target, row.Payload, status, key, row.Attempts, row.LeaseOwner, row.LeaseUntil)
	return err
}

// ClaimOutbox leases every currently-claimable row (not acked, and either unleased or with an expired lease)
// to owner for ttl, bumping attempts, and returns them. The single writer connection serializes claims, so
// two workers never both win the same row (S4 delivery lease). Rows are read fully before the UPDATE so the
// single connection is not held by an open cursor during the writes. If kinds is non-empty, ONLY rows of
// those kinds are claimed — a delivery worker must lease only the rows it actually delivers (so the job lane
// never grabs invalidation rows, and vice-versa); empty kinds claims every kind.
func (s *Store) ClaimOutbox(owner string, ttl time.Duration, kinds ...string) ([]OutboxRow, error) {
	now := time.Now().Unix()
	until := now + int64(ttl/time.Second)
	where := `status!='acked' AND (lease_owner='' OR lease_until<=?)`
	args := []any{now}
	if len(kinds) > 0 {
		ph := make([]string, len(kinds))
		for i, k := range kinds {
			ph[i] = "?"
			args = append(args, k)
		}
		where += ` AND kind IN (` + strings.Join(ph, ",") + `)`
	}
	var claimed []OutboxRow
	err := s.WithTx(func(tx *Tx) error {
		rows, err := tx.tx.Query(
			`SELECT id,kind,event_seq,target,payload,COALESCE(idempotency_key,''),attempts FROM outbox
			 WHERE `+where+` ORDER BY id`, args...)
		if err != nil {
			return err
		}
		var batch []OutboxRow
		for rows.Next() {
			var r OutboxRow
			if err := rows.Scan(&r.ID, &r.Kind, &r.EventSeq, &r.Target, &r.Payload, &r.IdempotencyKey, &r.Attempts); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, r)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		for i := range batch {
			r := &batch[i]
			if _, err := tx.tx.Exec(`UPDATE outbox SET status='claimed', lease_owner=?, lease_until=?, attempts=attempts+1 WHERE id=?`, owner, until, r.ID); err != nil {
				return err
			}
			r.Status, r.LeaseOwner, r.LeaseUntil, r.Attempts = "claimed", owner, until, r.Attempts+1
		}
		claimed = batch
		return nil
	})
	return claimed, err
}

// AckOutbox marks a delivered effect done. It must be acked by the lease holder (a stale worker whose lease
// expired and was reclaimed cannot ack the new holder's row) — an ack of a row not held by owner is an error.
func (s *Store) AckOutbox(id, owner string) error {
	res, err := s.db.Exec(`UPDATE outbox SET status='acked' WHERE id=? AND lease_owner=?`, id, owner)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("ack outbox %q: not held by %q", id, owner)
	}
	return nil
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
