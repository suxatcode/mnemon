# Sync ABI v1 (v1, FROZEN)

> Frozen 2026-06-12: the dual-replica e2e (run_sync_pair — push/pull roundtrip over TLS
> with attribution, offline I13, authn baseline) passed; per the stage-6 precondition the ABI
> freezes at stage close against TWO consumers (runtime co-hosted hub + mnemon-hub), not one.
> Naming (2026-06-12): the standalone hub binary, named `mnemond` when this ABI froze, builds as
> `mnemon-hub`; the `mnemond` name now belongs to the local governance daemon, which is not a
> consumer of this wire. Binary name only — no wire field, verb, or semantic changed.

The Remote Workspace sync wire: how a Local Mnemon replica pushes its accepted local commits to a
hub, pulls other replicas' commits back, and how both sides keep the attribution chain intact. The
hub is either a co-hosted Local Mnemon runtime (`mnemon-harness local run` serving `/sync/*`) or the
standalone `mnemon-hub` binary — ONE wire, two hostings.

Status: **FROZEN**. Freeze condition: the dual-replica e2e (`run_sync_pair`) passes. Until then field
additions are allowed only additively; nothing here is load-bearing for external integrators yet.

## 1. Wire verbs

Three verbs, named by `contract.SyncVerb*`:

| Verb          | HTTP                  | Purpose                                            |
|---------------|-----------------------|----------------------------------------------------|
| `sync.push`   | `POST /sync/push`     | submit a batch of local commits for adjudication   |
| `sync.pull`   | `POST /sync/pull`     | read other replicas' accepted commits after cursor |
| `sync.status` | `GET\|POST /sync/status` | hub-side sync evidence (counters, identity)     |

A replica credential grants ONLY these verbs — sync access never implies Agent Integration access
(observe/pull/status on the channel), and vice versa.

## 2. Authentication and grants (credential dual-form rule)

Identity is the authenticated principal (bearer token); the request body NEVER names identity.
A principal's sync access is a **replica grant**: `contract.ReplicaGrant{Principal, Token, Scopes}`.

The grant has exactly two on-disk forms with the SAME fields and semantics:

- **Co-hosted hub** (runtime): a `replica-agent` entry in the channel bindings file
  (`.mnemon/harness/channel/bindings.json`): `principal`, `credential_ref`, `subscription_scope`
  (= the grant scopes), `allowed_verbs` (the three sync verbs).
- **mnemon-hub**: an entry in `replicas.json`:

```json
{
  "schema_version": 1,
  "replicas": [
    {
      "principal": "replica-a@team",
      "credential_ref": "credentials/replica-a.token",
      "scopes": [ { "kind": "memory", "id": "project" } ]
    }
  ]
}
```

`replicas.json` rules: strict-decoded (unknown fields rejected); `credential_ref` is a bearer-token
file path, resolved relative to the replicas.json directory (or absolute); `scopes` MUST be
non-empty (fail closed — mnemon-hub refuses an empty grant); the file MUST NOT be world-readable
(mnemon-hub refuses to start; keep it 0600 in a 0700 directory, like the channel credential files).
Rotation = edit the credential file (or the entry) + restart mnemon-hub. The file is operator-supplied;
nothing writes it.

**Scope clamp.** There is ONE clamp implementation — `contract.ClampRefs` — shared by the channel
binding ceiling and the hub: empty requested defaults to the full granted scope; any explicit ref
outside the scope is an error; an EMPTY scope denies every explicit ref (fail closed). The hub
applies it on push (every commit's `ResourceRef` must clamp into the grant scope, else the commit is
rejected) and on pull (requested scopes clamp into the grant scope).

## 3. DTOs

JSON casing is the live wire form, documented as-is: the envelope fields are snake_case; the
`LocalCommit` / `ResourceRef` payloads marshal with Go field names (PascalCase). This asymmetry is a
recorded v1 fact, not a convention to imitate.

### LocalCommit (11 fields)

The append-only unit of sync: one accepted local decision's effect on one resource.

| JSON key          | Type        | Meaning                                                       |
|-------------------|-------------|---------------------------------------------------------------|
| `OriginReplicaID` | string      | the replica that produced the commit (who proposes)           |
| `LocalDecisionID` | string      | the origin's kernel decision id (idempotency key half)        |
| `LocalIngestSeq`  | int64       | the origin's durable ingest seq (based-on, origin ordering)   |
| `Actor`           | string      | the local principal whose write was accepted (who proposes)   |
| `CorrelationID`   | string      | the origin's correlation lineage                              |
| `ResourceRef`     | {Kind, ID}  | the governed resource written                                 |
| `ResourceVersion` | int64       | the origin's per-resource version after the write (based-on)  |
| `FieldsDigest`    | string      | sha256 hex of the canonical JSON of `Fields`                  |
| `Fields`          | object      | the full resource fields snapshot                             |
| `DecidedAt`       | string      | RFC3339, origin decision time (provenance only, never orders) |
| `Status`          | string      | local lifecycle: pending / synced / rejected / conflict       |

The commit identity (idempotency key) at the hub is
`(authenticated principal, OriginReplicaID, LocalDecisionID)`.

### SyncPushRequest / SyncPushResponse

```
SyncPushRequest  { "replica_id": string, "batch_id": string, "commits": [LocalCommit] }
SyncPushResponse { "accepted": [SyncCommitResult], "rejected": [SyncCommitResult],
                   "conflicts": [SyncCommitResult], "next_cursor": string (omitempty) }
```

`replica_id` MUST equal every commit's `OriginReplicaID` (a mismatch rejects the whole request).
`batch_id` is a client-computed digest of the batch (diagnostic provenance; the hub does not key on
it — per-commit idempotency is the replay defense).

### SyncCommitResult

```
{ "origin_replica_id": string, "local_decision_id": string,
  "resource_ref": {"Kind": string, "ID": string},
  "status": "accepted" | "rejected" | "conflict", "diagnostic": string (omitempty) }
```

### SyncPullRequest / SyncPullResponse

```
SyncPullRequest  { "replica_id": string, "remote_cursor": string, "scopes": [{"Kind","ID"}] }
SyncPullResponse { "commits": [LocalCommit], "diagnostics": [SyncCommitResult], "next_cursor": string }
```

`replica_id` is the puller's self-declared origin id, used ONLY to suppress echoing the puller's own
commits back; authorization comes from the principal's grant, never from this field. `remote_cursor`
is the decimal hub sequence the puller has consumed through ("" = 0 = from the beginning); `scopes`
narrows within the grant (clamped). Pulls serve at most 100 commits per call; `next_cursor` is the
new consume-through position (equal to the request cursor when nothing was served).

### SyncStatusRequest / SyncStatusResponse

There is no request DTO: `sync.status` carries no body; identity is the credential.

```
SyncStatusResponse {
  "principal": string,              // authenticated principal (echo)
  "remote_workspace": "connected",
  "hub_commits_received": int64,    // total commits accepted into the hub log
  "hub_commits_served": int64,      // total commits returned across all pulls
  "hub_replica_cursors": { string: string }  // principal -> last next_cursor served to it (omitempty)
}
```

The three `hub_*` fields are the v1 hub counters — an ADDITIVE DTO change; a pre-counter hub simply
omits them (clients must treat absent as zero).

## 4. Hub adjudication semantics

The hub keeps an **append-only event log** of accepted commits (`sync_remote_commits`, monotonically
sequenced by `remote_seq`). Push adjudicates per commit:

- **accepted** — first sight of `(principal, OriginReplicaID, LocalDecisionID)` and the commit
  validates: provenance fields present, resource ref present, kind ∈ `contract.SyncableResourceKinds`
  (`memory`, `skill` — the SAME set the local decision sink uses to produce commits, shared so the
  accept surface and the produce surface cannot drift), fields present, `FieldsDigest` matches
  `Fields`, and the ref clamps into the grant scope. The commit is appended; `next_cursor` advances.
- **rejected** — a validation or scope-clamp failure; `diagnostic` names the reason. Nothing is
  appended; a rejected commit may be corrected and re-pushed under a NEW decision id.
- **conflict** — idempotency-key reuse with different content ONLY: the key was seen before but the
  commit body differs (`diagnostic`: `"sync idempotency key reused with different commit"`). Nothing
  is appended or overwritten — the log is append-only and first-write-wins per key.

**Replay idempotency**: re-pushing an identical batch returns the same per-commit `accepted` results
and appends ZERO new rows. Pull replay is idempotent by cursor: re-pulling from an old cursor
re-serves the same commits in the same order; the puller's import dedupe (see §6) absorbs them.

## 5. Explicit deviation (recorded, not hidden)

> MVP hub = event log + per-commit adjudication, **NO remote reducer**; conflict adjudication
> happens local-side at import (kernel CAS); reducer deferred until a remote consumer exists.

"Isomorphic with local" in v1 means the SAME event-sourced semantics and attribution fields on both
ledgers — not a kernel running in the hub. The hub never materializes resources, never versions
them, never merges: two replicas' divergent writes to the same entry both LAND in the hub log and
the divergence is adjudicated at each puller's import (kernel CAS + import rules), where it leaves a
durable diagnostic. A hub-side version conflict would be new semantics and is out of v1.

## 6. Puller-side import (what the cursor and dedupe keys promise)

Pulled commits re-enter the puller's Event Intake under the well-known principal `sync@local`
(`contract.SyncImportActor`) — never bypassing the kernel. Exactly-once is the intake dedupe over
the six-part key:

```
ExternalID = "pull:<remote_id>:<OriginReplicaID>:<LocalDecisionID>:<Kind>:<ID>"
```

- An importable kind (`memory`, `skill`) ingests its `remote.<kind>.commit_observed` event; the
  import rule merges non-conflicting entries and DENIES a same-id/different-content divergence with
  a durable `*.diagnostic` (the local half of the attribution chain).
- A kind with **no import mapping** (a newer hub serving a kind this replica cannot import) ingests
  `sync.import_skipped.observed` with `ExternalID = <six-part key> + ":skipped"` and payload
  `{kind, origin_replica_id, local_decision_id, remote_id}`; a deny rule in the sync-import rule set
  turns it into a durable `sync.diagnostic` naming the kind. Exactly-once: a re-pull is a dedupe hit
  and does not duplicate the diagnostic. The pull cursor still advances — the skip is visible, never
  silent, and never wedges the stream.

The pull cursor is durable per remote (`sync_pull:<remote_id>`), advanced only after the batch is
imported.

## 7. Attribution field map

| Question (chain link)        | Fields                                                                                       |
|------------------------------|----------------------------------------------------------------------------------------------|
| Who proposed                 | `LocalCommit.Actor` (local principal) + `OriginReplicaID` (which replica)                    |
| Which authority accepted     | hub adjudication (`sync_remote_commits.status` + `SyncCommitResult.Status`) and, at import, the puller's kernel decision under `sync@local` |
| Based on                     | `LocalIngestSeq` (origin log position) + `ResourceVersion` (origin per-resource version)     |
| Why refused                  | `SyncCommitResult.Diagnostic` (hub side) / decision `Reason` + durable `*.diagnostic` event (import side), joined back via `CausedBy`/`CorrelationID` |

Both ledgers carry the chain: the pusher's `sync_commits` row mirrors the hub verdict
(status/diagnostic/remote peer/acked_at); the hub row carries origin identity + receive time; the
puller's import decision carries the same origin identity through the event payload.

## 8. T2 boundary honesty

- Transport auth is a **bearer token** over TLS. mnemon-hub serves TLS natively (`--tls-cert/--tls-key`);
  `--dev-selfsigned` generates a dev/e2e certificate pair — this is honest dev tooling, not a
  production PKI story.
- Clients refuse a plaintext `http://` endpoint with a non-loopback host unless explicitly
  overridden (`--allow-insecure-remote`); `sync connect` enforces the same gate at write time.
  `remotes.json` may carry an optional `ca_file` (PEM bundle, resolved relative to the project root)
  pinning the remote's TLS root.
- **Token replay** is T1-equivalent semantics: whoever holds the token IS the principal, exactly as
  with the local channel credentials. TLS protects the token in transit; the file permissions
  protect it at rest; there is no per-request nonce in v1.
- **Batch replay** is idempotent by design (§4) — replaying a captured push cannot duplicate or
  mutate hub state; replaying a pull yields data the credential was already entitled to.
- mnemon-hub emits one audit line per request to stdout: timestamp, principal, verb, result. `result`
  is the **request-level** outcome only — `unauthorized` (401), `bad_request` (400 — malformed JSON,
  a missing/invalid field, or a disallowed HTTP method), `denied` (403 — no replica grant or an
  out-of-scope clamp), or `ok`. The PER-COMMIT `accepted` / `rejected` / `conflict` verdicts ride
  the `200` response body, never the audit line: a `sync.push` whose every commit is rejected still
  audits `result=ok` because the request itself parsed and was authorized.

## 9. Hub store ownership

One mnemon-hub per hub store: `mnemon-hub` opens its SQLite store with the same single-writer flock the
local store uses. Concurrent pushes from multiple replicas are serialized by the store transaction
(single connection); both land, in arrival order.

## 10. Freeze

This document freezes (FROZEN marker removed) when the dual-replica e2e (`run_sync_pair`: A pushes,
B pulls, conflict attribution on both ledgers, offline leg, security leg) passes. Until then it is
kept in lockstep with the implementation by the stage-6 tasks.
