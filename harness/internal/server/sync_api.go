package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

func (r *Runtime) SyncPush(principal contract.ActorID, req contract.SyncPushRequest) (contract.SyncPushResponse, error) {
	if _, err := r.requireSyncBinding(principal, VerbSyncPush); err != nil {
		return contract.SyncPushResponse{}, err
	}
	replicaID := strings.TrimSpace(req.ReplicaID)
	if replicaID == "" {
		return contract.SyncPushResponse{}, fmt.Errorf("sync push requires replica_id")
	}
	var resp contract.SyncPushResponse
	for _, commit := range req.Commits {
		if commit.OriginReplicaID != replicaID {
			return contract.SyncPushResponse{}, fmt.Errorf("sync push replica_id %q does not match commit origin %q", replicaID, commit.OriginReplicaID)
		}
		if diagnostic := validateSyncCommit(commit); diagnostic != "" {
			resp.Rejected = append(resp.Rejected, syncResult(commit, "rejected", diagnostic))
			continue
		}
		rec, err := r.store.RecordRemoteSyncCommit(string(principal), commit, r.cs.now())
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

func (r *Runtime) SyncPull(principal contract.ActorID, req contract.SyncPullRequest) (contract.SyncPullResponse, error) {
	b, err := r.requireSyncBinding(principal, VerbSyncPull)
	if err != nil {
		return contract.SyncPullResponse{}, err
	}
	replicaID := strings.TrimSpace(req.ReplicaID)
	if replicaID == "" {
		return contract.SyncPullResponse{}, fmt.Errorf("sync pull requires replica_id")
	}
	cursor := int64(0)
	if strings.TrimSpace(req.RemoteCursor) != "" {
		cursor, err = strconv.ParseInt(req.RemoteCursor, 10, 64)
		if err != nil {
			return contract.SyncPullResponse{}, fmt.Errorf("parse remote_cursor: %w", err)
		}
	}
	scopes, err := clampSyncScopes(b, req.Scopes)
	if err != nil {
		return contract.SyncPullResponse{}, err
	}
	records, next, err := r.store.RemoteSyncCommitsAfter(cursor, replicaID, scopes, 100)
	if err != nil {
		return contract.SyncPullResponse{}, err
	}
	resp := contract.SyncPullResponse{NextCursor: strconv.FormatInt(next, 10)}
	for _, rec := range records {
		resp.Commits = append(resp.Commits, rec.Commit)
	}
	return resp, nil
}

func (r *Runtime) SyncStatus(principal contract.ActorID) (contract.SyncStatusResponse, error) {
	if _, err := r.requireSyncBinding(principal, VerbSyncStatus); err != nil {
		return contract.SyncStatusResponse{}, err
	}
	return contract.SyncStatusResponse{Principal: principal, RemoteWorkspace: "connected"}, nil
}

func (r *Runtime) requireSyncBinding(principal contract.ActorID, verb Verb) (ChannelBinding, error) {
	if r.bindings == nil {
		return ChannelBinding{}, fmt.Errorf("sync requires a replica-agent binding")
	}
	b, ok := r.bindings.Binding(principal)
	if !ok {
		return ChannelBinding{}, fmt.Errorf("no channel binding for principal %q", principal)
	}
	if b.ActorKind != contract.KindReplicaAgent {
		return ChannelBinding{}, fmt.Errorf("principal %q is not a replica-agent", principal)
	}
	if !b.Allows(verb) {
		return ChannelBinding{}, fmt.Errorf("principal %q is not bound to %s", principal, verb)
	}
	return b, nil
}

func validateSyncCommit(commit contract.LocalCommit) string {
	switch {
	case strings.TrimSpace(commit.OriginReplicaID) == "":
		return "origin_replica_id is required"
	case strings.TrimSpace(commit.LocalDecisionID) == "":
		return "local_decision_id is required"
	case strings.TrimSpace(string(commit.ResourceRef.Kind)) == "" || strings.TrimSpace(string(commit.ResourceRef.ID)) == "":
		return "resource_ref is required"
	case !syncableResourceKinds[commit.ResourceRef.Kind]:
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

func clampSyncScopes(binding ChannelBinding, requested []contract.ResourceRef) ([]contract.ResourceRef, error) {
	if len(requested) == 0 {
		return append([]contract.ResourceRef(nil), binding.SubscriptionScope...), nil
	}
	if len(binding.SubscriptionScope) == 0 {
		return append([]contract.ResourceRef(nil), requested...), nil
	}
	allowed := make(map[contract.ResourceRef]bool, len(binding.SubscriptionScope))
	for _, ref := range binding.SubscriptionScope {
		allowed[ref] = true
	}
	for _, ref := range requested {
		if !allowed[ref] {
			return nil, fmt.Errorf("sync scope %s/%s is outside replica binding scope", ref.Kind, ref.ID)
		}
	}
	return append([]contract.ResourceRef(nil), requested...), nil
}

func syncCommitFieldsDigest(fields map[string]any) string {
	b, _ := json.Marshal(fields)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
