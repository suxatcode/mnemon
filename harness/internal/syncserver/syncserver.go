// Package syncserver is the hub half of Remote Workspace sync (sync-abi-v1): push adjudication
// against the append-only sync_remote_commits log, pull serving with the ONE scope clamp, and the
// status counters. It is extracted from the runtime so the standalone hub binary (mnemon-hub) can host
// the same wire without the runtime: it imports ONLY contract + store (+stdlib) — never channel /
// runtime / app / hostsurface (the trust-domain import boundary, pinned by a test). Replica
// authorization enters through the Grants seam; the co-hosted runtime adapts its channel bindings to
// grants, mnemon-hub builds grants from replicas.json — same fields, same semantics (dual-form rule).
package syncserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

// ReplicaGrant aliases the contract type (the grant record is ABI surface, sync-abi-v1 §2).
type ReplicaGrant = contract.ReplicaGrant

// BadRequestError marks a request-VALIDATION failure (a malformed/missing field) as distinct from an
// AUTHORIZATION failure (no grant / out-of-scope). The HTTP layer maps it to 400; everything else
// from Push/Pull/Status (no replica grant, out-of-scope clamp) stays 403 (LOW-10). It is the wire
// layer's only error-class signal — the per-commit accept/reject/conflict verdicts ride the 200 body.
type BadRequestError struct{ err error }

func (e *BadRequestError) Error() string { return e.err.Error() }
func (e *BadRequestError) Unwrap() error { return e.err }

// badRequestf builds a BadRequestError (validation class).
func badRequestf(format string, a ...any) error {
	return &BadRequestError{err: fmt.Errorf(format, a...)}
}

// IsBadRequest reports whether err is (or wraps) a request-validation failure.
func IsBadRequest(err error) bool {
	var bre *BadRequestError
	return errors.As(err, &bre)
}

// Grants resolves an authenticated principal's replica grant for ONE sync verb (a contract.SyncVerb*
// string). Implementations MUST be fail-closed: an unknown principal, a non-replica principal, or an
// ungranted verb returns false — there is no anonymous or default grant.
type Grants interface {
	Grant(principal contract.ActorID, verb string) (ReplicaGrant, bool)
}

// GrantMap is the static Grants form mnemon-hub builds from replicas.json: every listed replica holds
// all three sync verbs (a replica credential is sync-only by construction; per-verb narrowing is the
// co-hosted binding form's concern).
type GrantMap map[contract.ActorID]ReplicaGrant

func (m GrantMap) Grant(principal contract.ActorID, verb string) (ReplicaGrant, bool) {
	if verb != contract.SyncVerbPush && verb != contract.SyncVerbPull && verb != contract.SyncVerbStatus {
		return ReplicaGrant{}, false
	}
	g, ok := m[principal]
	return g, ok
}

// hub-side durable cursor names (bookkeeping for the status counters; never an ordering source).
const (
	serveCursorPrefix = "sync_serve:"       // + principal -> last next_cursor served to it
	servedTotalCursor = "sync_served_total" // total commits returned across all pulls
)

// Server is one hub over one open store. It holds no other state: adjudication and counters are
// durable in the store, so a restart (or a concurrent co-hosted runtime surface) sees the same hub.
type Server struct {
	store  *store.Store
	grants Grants
	now    func() string
}

// New wires a hub Server over an OPEN store (the caller owns the store's single-writer lock and its
// lifecycle). now stamps received_at on accepted commits.
func New(st *store.Store, grants Grants, now func() string) *Server {
	return &Server{store: st, grants: grants, now: now}
}

// Push adjudicates one batch per commit (sync-abi-v1 §4): accepted (appended, first sight of the
// idempotency key), rejected (validation or scope-clamp failure, with diagnostic), or conflict
// (idempotency-key reuse with DIFFERENT content only). Replaying an identical batch repeats the
// accepted results and appends nothing. The request replica_id must match every commit's origin —
// a mismatch rejects the whole request before any adjudication.
func (s *Server) Push(principal contract.ActorID, req contract.SyncPushRequest) (contract.SyncPushResponse, error) {
	grant, ok := s.grants.Grant(principal, contract.SyncVerbPush)
	if !ok {
		return contract.SyncPushResponse{}, fmt.Errorf("principal %q has no replica grant for %s", principal, contract.SyncVerbPush)
	}
	replicaID := strings.TrimSpace(req.ReplicaID)
	if replicaID == "" {
		return contract.SyncPushResponse{}, badRequestf("sync push requires replica_id")
	}
	var resp contract.SyncPushResponse
	for _, commit := range req.Commits {
		if commit.OriginReplicaID != replicaID {
			return contract.SyncPushResponse{}, badRequestf("sync push replica_id %q does not match commit origin %q", replicaID, commit.OriginReplicaID)
		}
		if diagnostic := validateSyncCommit(commit); diagnostic != "" {
			resp.Rejected = append(resp.Rejected, syncResult(commit, "rejected", diagnostic))
			continue
		}
		// The ONE clamp (contract.ClampRefs), fail-closed: a commit outside the grant scope is
		// rejected per-commit with the clamp's diagnostic — the push-side twin of the pull clamp.
		if _, err := contract.ClampRefs(principal, grant.Scopes, []contract.ResourceRef{commit.ResourceRef}); err != nil {
			resp.Rejected = append(resp.Rejected, syncResult(commit, "rejected", err.Error()))
			continue
		}
		rec, err := s.store.RecordRemoteSyncCommit(string(principal), commit, s.now())
		if err != nil {
			return contract.SyncPushResponse{}, err
		}
		switch rec.Status {
		case "accepted":
			resp.Accepted = append(resp.Accepted, syncResult(rec.Commit, "accepted", ""))
			resp.NextCursor = strconv.FormatInt(rec.RemoteSeq, 10)
		case "conflict":
			resp.Conflicts = append(resp.Conflicts, syncResult(commit, "conflict", rec.Diagnostic))
		default:
			resp.Rejected = append(resp.Rejected, syncResult(commit, rec.Status, rec.Diagnostic))
		}
	}
	return resp, nil
}

// Pull serves accepted commits after the request cursor, excluding the puller's own origin and
// clamped to the grant scope (requested scopes may only narrow). It also advances the hub's serve
// bookkeeping (last cursor per principal + served total) for the status counters.
func (s *Server) Pull(principal contract.ActorID, req contract.SyncPullRequest) (contract.SyncPullResponse, error) {
	grant, ok := s.grants.Grant(principal, contract.SyncVerbPull)
	if !ok {
		return contract.SyncPullResponse{}, fmt.Errorf("principal %q has no replica grant for %s", principal, contract.SyncVerbPull)
	}
	replicaID := strings.TrimSpace(req.ReplicaID)
	if replicaID == "" {
		return contract.SyncPullResponse{}, badRequestf("sync pull requires replica_id")
	}
	cursor := int64(0)
	if strings.TrimSpace(req.RemoteCursor) != "" {
		var err error
		cursor, err = strconv.ParseInt(req.RemoteCursor, 10, 64)
		if err != nil {
			return contract.SyncPullResponse{}, badRequestf("parse remote_cursor: %v", err)
		}
	}
	scopes, err := contract.ClampRefs(principal, grant.Scopes, req.Scopes)
	if err != nil {
		return contract.SyncPullResponse{}, fmt.Errorf("sync scope: %w", err)
	}
	records, next, err := s.store.RemoteSyncCommitsAfter(cursor, replicaID, scopes, 100)
	if err != nil {
		return contract.SyncPullResponse{}, err
	}
	resp := contract.SyncPullResponse{NextCursor: strconv.FormatInt(next, 10)}
	for _, rec := range records {
		resp.Commits = append(resp.Commits, rec.Commit)
	}
	// Serve bookkeeping for the status counters; best-effort durability is fine (a lost update
	// understates a counter, never corrupts adjudication), but the write errors still surface.
	if len(records) > 0 {
		if err := s.store.SetCursor(servedTotalCursor, s.store.GetCursor(servedTotalCursor)+int64(len(records))); err != nil {
			return contract.SyncPullResponse{}, err
		}
	}
	if err := s.store.SetCursor(serveCursorPrefix+string(principal), next); err != nil {
		return contract.SyncPullResponse{}, err
	}
	return resp, nil
}

// Status reports the hub-side counters (sync-abi-v1 §3, additive): commits received (= rows in the
// append-only log), commits served across pulls, and the last cursor served per replica principal.
func (s *Server) Status(principal contract.ActorID) (contract.SyncStatusResponse, error) {
	if _, ok := s.grants.Grant(principal, contract.SyncVerbStatus); !ok {
		return contract.SyncStatusResponse{}, fmt.Errorf("principal %q has no replica grant for %s", principal, contract.SyncVerbStatus)
	}
	received, err := s.store.RemoteSyncCommitCount()
	if err != nil {
		return contract.SyncStatusResponse{}, err
	}
	cursors, err := s.store.CursorsByPrefix(serveCursorPrefix)
	if err != nil {
		return contract.SyncStatusResponse{}, err
	}
	resp := contract.SyncStatusResponse{
		Principal:          principal,
		RemoteWorkspace:    "connected",
		HubCommitsReceived: received,
		HubCommitsServed:   s.store.GetCursor(servedTotalCursor),
	}
	if len(cursors) > 0 {
		resp.HubReplicaCursors = make(map[string]string, len(cursors))
		for name, seq := range cursors {
			resp.HubReplicaCursors[name] = strconv.FormatInt(seq, 10)
		}
	}
	return resp, nil
}

// validateSyncCommit is the hub's per-commit validation (sync-abi-v1 §4): provenance, resource ref,
// kind in the shared syncable set, fields present, digest matching. Returns "" when valid, else the
// rejection diagnostic.
func validateSyncCommit(commit contract.LocalCommit) string {
	switch {
	case strings.TrimSpace(commit.OriginReplicaID) == "":
		return "origin_replica_id is required"
	case strings.TrimSpace(commit.LocalDecisionID) == "":
		return "local_decision_id is required"
	case strings.TrimSpace(string(commit.Actor)) == "":
		return "actor is required"
	case strings.TrimSpace(string(commit.ResourceRef.Kind)) == "" || strings.TrimSpace(string(commit.ResourceRef.ID)) == "":
		return "resource_ref is required"
	case !contract.SyncableResourceKinds[commit.ResourceRef.Kind]:
		return fmt.Sprintf("resource kind %q is not syncable", commit.ResourceRef.Kind)
	case commit.Fields == nil:
		return "fields are required"
	case strings.TrimSpace(commit.FieldsDigest) == "":
		return "fields_digest is required"
	case commit.FieldsDigest != syncCommitFieldsDigest(commit.Fields):
		return "fields_digest does not match fields"
	default:
		return ""
	}
}

func syncResult(commit contract.LocalCommit, status, diagnostic string) contract.SyncCommitResult {
	return contract.SyncCommitResult{
		OriginReplicaID: commit.OriginReplicaID,
		LocalDecisionID: commit.LocalDecisionID,
		ResourceRef:     commit.ResourceRef,
		Status:          status,
		Diagnostic:      diagnostic,
	}
}

func syncCommitFieldsDigest(fields map[string]any) string {
	b, _ := json.Marshal(fields)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
