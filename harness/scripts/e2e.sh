#!/usr/bin/env bash
# End-to-end system acceptance: the full hot path (setup -> local run -> observe(EventDraft) ->
# channel -> intake -> synchronous tick -> rule -> kernel -> projection -> pull/status), plus the
# negative diagnostic case and the refresh no-clobber, for BOTH hosts (codex + claude-code).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

WORK="$(mktemp -d)"
MH="$WORK/mnemon-harness"
PIDFILE="$WORK/run.pid"
cleanup() {
	[ -f "$PIDFILE" ] && kill "$(cat "$PIDFILE")" 2>/dev/null || true
	# the sync-pair stanza runs three background processes; reap any survivor on ANY exit path
	local f
	for f in "$WORK"/*.pid; do
		[ -f "$f" ] && kill "$(cat "$f")" 2>/dev/null || true
	done
	pkill -f "$WORK/mnemond" 2>/dev/null || true
	rm -rf "$WORK"
}
trap cleanup EXIT

echo "building mnemon-harness..."
go build -o "$MH" ./harness/cmd/mnemon-harness

fail() {
	echo "E2E FAIL ($CUR_HOST): $1" >&2
	exit 1
}

run_host() {
	local host="$1" principal="$2" port="$3" configdir="$4"
	CUR_HOST="$host"
	local proj="$WORK/proj-$host"
	mkdir -p "$proj"
	echo "=== E2E host=$host port=$port ==="
	(
		cd "$proj"
		local addr="http://127.0.0.1:$port"
		local tok=".mnemon/harness/channel/credentials/$(printf '%s' "$principal" | tr '@' '-').token"

		"$MH" setup --host "$host" --memory --principal "$principal" --control-url "$addr" >/dev/null

		# start Local Mnemon (creates governed.db on first serve)
		"$MH" local run >"$WORK/run-$host.log" 2>&1 &
		local runpid=$!
		echo "$runpid" >"$PIDFILE"

		# wait until the channel answers a status call
		local up=0 i
		for i in $(seq 1 60); do
			if "$MH" control status --addr "$addr" --principal "$principal" --token-file "$tok" >/dev/null 2>&1; then
				up=1
				break
			fi
			sleep 0.1
		done
		[ "$up" = 1 ] || { cat "$WORK/run-$host.log"; exit 1; }

		# observe a valid candidate -> synchronous tick admits -> kernel applies
		local out
		out="$("$MH" control observe --addr "$addr" --principal "$principal" --token-file "$tok" \
			--type memory.write_candidate.observed --external-id m1 \
			--payload '{"content":"E2E memory works for '"$host"'","source":"user","confidence":"high"}')"
		case "$out" in *ticked=true*) ;; *) echo "observe: $out"; exit 1 ;; esac

		# pull returns the memory (one resource)
		out="$("$MH" control pull --addr "$addr" --principal "$principal" --token-file "$tok")"
		case "$out" in *resources=1*) ;; *) echo "pull: $out"; exit 1 ;; esac

		# status digest non-empty
		out="$("$MH" control status --addr "$addr" --principal "$principal" --token-file "$tok")"
		case "$out" in *digest=[0-9a-f]*) ;; *) echo "status: $out"; exit 1 ;; esac

		# negative: a secret-like candidate is denied; pull still shows exactly one resource
		"$MH" control observe --addr "$addr" --principal "$principal" --token-file "$tok" \
			--type memory.write_candidate.observed --external-id bad1 \
			--payload '{"content":"api_key=sk-abcdefABCDEF123456","source":"user","confidence":"high"}' >/dev/null
		out="$("$MH" control pull --addr "$addr" --principal "$principal" --token-file "$tok")"
		case "$out" in *resources=1*) ;; *) echo "negative pull leaked: $out"; exit 1 ;; esac

		# 阶段一:写入即见 —— 不跑任何 prime,driver 在 invalidation 后自动再生镜像。
		# (m2 deliberately keeps the legacy underscore type: the standing ALIAS PIN.)
		"$MH" control observe --addr "$addr" --principal "$principal" --token-file "$tok" \
			--type memory.write_candidate_observed --external-id m2 \
			--payload '{"content":"E2E driver mirror '"$host"'","source":"user","confidence":"high"}' >/dev/null
		local mirror="$configdir/mnemon-memory/MEMORY.md" seen=0
		for i in $(seq 1 100); do
			if grep -q "E2E driver mirror $host" "$mirror" 2>/dev/null; then
				seen=1
				break
			fi
			sleep 0.1
		done
		[ "$seen" = 1 ] || { echo "driver did not regenerate the mirror within 10s"; exit 1; }

		# refresh no-clobber: hand-edit a projected GUIDE, refresh, assert the edit is preserved + reported
		local guide="$configdir/mnemon-memory/GUIDE.md"
		printf '# E2E USER EDIT\n\n%s' "$(cat "$guide")" >"$guide.tmp" && mv "$guide.tmp" "$guide"
		out="$("$MH" refresh --host "$host" --memory)"
		case "$out" in *GUIDE.md*) ;; *) echo "refresh did not report GUIDE: $out"; exit 1 ;; esac
		grep -q "E2E USER EDIT" "$guide" || { echo "refresh clobbered GUIDE"; exit 1; }

		# stop Local Mnemon and reap it quietly (releases the port + the store lock before the next host)
		{ kill "$runpid" 2>/dev/null; wait "$runpid"; } 2>/dev/null || true
		rm -f "$PIDFILE"
	) || fail "host flow failed (see $WORK/run-$host.log)"
	sleep 0.3
	echo "    host=$host OK"
}

# run_skill exercises the SKILL loop end-to-end (the memory arm above covers the memory loop): setup
# --skills, observe a skill candidate, tick, pull.
run_skill() {
	local host="$1" principal="$2" addr="http://127.0.0.1:8787"
	CUR_HOST="$host-skill"
	local proj="$WORK/proj-skill-$host"
	mkdir -p "$proj"
	echo "=== E2E skill loop ($host) ==="
	(
		cd "$proj"
		local tok=".mnemon/harness/channel/credentials/$(printf '%s' "$principal" | tr '@' '-').token"
		"$MH" setup --host "$host" --skills --principal "$principal" --control-url "$addr" >/dev/null
		"$MH" local run >"$WORK/run-skill.log" 2>&1 &
		local runpid=$!
		echo "$runpid" >"$PIDFILE"
		local up=0 i
		for i in $(seq 1 60); do
			if "$MH" control status --addr "$addr" --principal "$principal" --token-file "$tok" >/dev/null 2>&1; then
				up=1
				break
			fi
			sleep 0.1
		done
		[ "$up" = 1 ] || { cat "$WORK/run-skill.log"; exit 1; }

		local out
		out="$("$MH" control observe --addr "$addr" --principal "$principal" --token-file "$tok" \
			--type skill.write_candidate.observed --external-id s1 \
			--payload '{"skill_id":"e2e-skill","name":"E2E Skill","status":"active","source":"user","confidence":"high"}')"
		case "$out" in *ticked=true*) ;; *) echo "skill observe: $out"; exit 1 ;; esac
		out="$("$MH" control pull --addr "$addr" --principal "$principal" --token-file "$tok")"
		case "$out" in *resources=1*) ;; *) echo "skill pull: $out"; exit 1 ;; esac

		{ kill "$runpid" 2>/dev/null; wait "$runpid"; } 2>/dev/null || true
		rm -f "$PIDFILE"
	) || fail "skill flow failed (see $WORK/run-skill.log)"
	sleep 0.3
	echo "    skill loop ($host) OK"
}

# run_note proves the platform claim on the PRODUCT path (note AND the 4th capability decision):
# a capability whose descriptor +
# KindCatalog entry exist in code (note) stands up via CONFIG EDIT ALONE — no new Go in app/cmd.
# setup fail-closes `--loop note` (note has no host assets, correctly), so the stanza does what a
# platform operator would: edit the setup-written config.json loops list + bindings.json scope.
run_note() {
	local principal="codex@project" addr="http://127.0.0.1:8787"
	CUR_HOST="note-via-config"
	local proj="$WORK/proj-note"
	mkdir -p "$proj"
	echo "=== E2E note capability via config alone ==="
	(
		cd "$proj"
		local tok=".mnemon/harness/channel/credentials/codex-project.token"
		"$MH" setup --host codex --memory --principal "$principal" --control-url "$addr" >/dev/null

		# The config edit: enable the note loop + widen the binding to the note type/scope.
		python3 - <<-'PYEOF'
		import json
		cfg = json.load(open(".mnemon/harness/local/config.json"))
		cfg["loops"].append("note")
		cfg["loops"].append("decision")
		json.dump(cfg, open(".mnemon/harness/local/config.json", "w"), indent=2)
		doc = json.load(open(".mnemon/harness/channel/bindings.json"))
		b = doc["bindings"][0]
		b["allowed_observed_types"].append("note.write_candidate.observed")
		b["subscription_scope"].append({"kind": "note", "id": "project"})
		b["allowed_observed_types"].append("decision.write_candidate.observed")
		b["subscription_scope"].append({"kind": "decision", "id": "project"})
		json.dump(doc, open(".mnemon/harness/channel/bindings.json", "w"), indent=2)
		PYEOF

		"$MH" local run >"$WORK/run-note.log" 2>&1 &
		local runpid=$!
		echo "$runpid" >"$PIDFILE"
		local up=0 i
		for i in $(seq 1 60); do
			if "$MH" control status --addr "$addr" --principal "$principal" --token-file "$tok" >/dev/null 2>&1; then
				up=1
				break
			fi
			sleep 0.1
		done
		[ "$up" = 1 ] || { cat "$WORK/run-note.log"; exit 1; }

		# `resources=N` counts SCOPED refs (version-0 included), so it cannot prove existence.
		# The content digest folds Kind:ID:Version+fields per scoped ref: an admitted note write
		# necessarily changes it. ticked=true + digest delta = the note landed.
		local out pre post
		out="$("$MH" control pull --addr "$addr" --principal "$principal" --token-file "$tok")"
		pre="${out##*digest=}"; pre="${pre%% *}"
		out="$("$MH" control observe --addr "$addr" --principal "$principal" --token-file "$tok" \
			--type note.write_candidate.observed --external-id n1 \
			--payload '{"text":"note stands up via config alone"}')"
		case "$out" in *ticked=true*) ;; *) echo "note observe: $out"; exit 1 ;; esac
		out="$("$MH" control pull --addr "$addr" --principal "$principal" --token-file "$tok")"
		post="${out##*digest=}"; post="${post%% *}"
		[ -n "$pre" ] && [ -n "$post" ] && [ "$pre" != "$post" ] || { echo "note write did not change the scoped digest (pre=$pre post=$post)"; exit 1; }

		# 阶段二:第四能力 decision —— spec 文件 + KindCatalog/SchemaGuard 一行,零新增行为代码。
		out="$("$MH" control observe --addr "$addr" --principal "$principal" --token-file "$tok" \
			--type decision.write_candidate.observed --external-id d1 \
			--payload '{"text":"decision stands up from a spec file"}')"
		case "$out" in *ticked=true*) ;; *) echo "decision observe: $out"; exit 1 ;; esac
		out="$("$MH" control pull --addr "$addr" --principal "$principal" --token-file "$tok")"
		post2="${out##*digest=}"; post2="${post2%% *}"
		[ -n "$post2" ] && [ "$post2" != "$post" ] || { echo "decision write did not change the scoped digest"; exit 1; }

		{ kill "$runpid" 2>/dev/null; wait "$runpid"; } 2>/dev/null || true
		rm -f "$PIDFILE"
	) || fail "note flow failed (see $WORK/run-note.log)"
	sleep 0.3
	echo "    note via config alone OK"
}

# run_external_goal proves stage 5 on the product path: a capability NEVER embedded (goal) stands
# up from a pure external package directory (.mnemon/loops/goal/capability.json) + the SAME
# config.loops/binding edit note/decision use — admission-equal rights. Includes the governed pull
# CONTENT leg (the goal text arrives via the pull verb, not only a digest delta) and the negative
# path: a malformed second package REFUSES `local run` boot, naming its path on stderr.
run_external_goal() {
	local principal="codex@project" addr="http://127.0.0.1:8787"
	CUR_HOST="external-goal"
	local proj="$WORK/proj-external-goal"
	mkdir -p "$proj"
	echo "=== E2E external goal capability package ==="
	(
		cd "$proj"
		local tok=".mnemon/harness/channel/credentials/codex-project.token"
		"$MH" setup --host codex --memory --principal "$principal" --control-url "$addr" >/dev/null

		# The external package: directory presence = capability declaration (loop-package-v1,
		# "External capability packages").
		mkdir -p .mnemon/loops/goal
		cat >.mnemon/loops/goal/capability.json <<-'JSONEOF'
		{
		  "schema_version": 1,
		  "name": "goal",
		  "observed_type": "goal.write_candidate.observed",
		  "proposed_type": "goal.write.proposed",
		  "resource_kind": "goal",
		  "items_field": "items",
		  "fields": [
		    {
		      "name": "statement",
		      "validators": [
		        {"id": "required", "params": {"missing_style": "empty"}},
		        {"id": "safety:unsafe"}
		      ]
		    }
		  ],
		  "render": {
		    "content": {"member": "bullet-list", "params": {"title": "# Goals", "field": "statement"}},
		    "static": {"statement": "project"}
		  }
		}
		JSONEOF

		# The enablement edit — EXACTLY isomorphic to note/decision: config.loops + binding
		# scope/types (config.loops stays the product-path authority).
		python3 - <<-'PYEOF'
		import json
		cfg = json.load(open(".mnemon/harness/local/config.json"))
		cfg["loops"].append("goal")
		json.dump(cfg, open(".mnemon/harness/local/config.json", "w"), indent=2)
		doc = json.load(open(".mnemon/harness/channel/bindings.json"))
		b = doc["bindings"][0]
		b["allowed_observed_types"].append("goal.write_candidate.observed")
		b["subscription_scope"].append({"kind": "goal", "id": "project"})
		json.dump(doc, open(".mnemon/harness/channel/bindings.json", "w"), indent=2)
		PYEOF

		"$MH" local run >"$WORK/run-external-goal.log" 2>&1 &
		local runpid=$!
		echo "$runpid" >"$PIDFILE"
		local up=0 i
		for i in $(seq 1 60); do
			if "$MH" control status --addr "$addr" --principal "$principal" --token-file "$tok" >/dev/null 2>&1; then
				up=1
				break
			fi
			sleep 0.1
		done
		[ "$up" = 1 ] || { cat "$WORK/run-external-goal.log"; exit 1; }

		# observe -> synchronous tick admits through the EXTERNAL rule (goal is not embedded, so
		# there is no builtin fallback that could fake this) -> scoped digest delta.
		local out pre post
		out="$("$MH" control pull --addr "$addr" --principal "$principal" --token-file "$tok")"
		pre="${out##*digest=}"; pre="${pre%% *}"
		out="$("$MH" control observe --addr "$addr" --principal "$principal" --token-file "$tok" \
			--type goal.write_candidate.observed --external-id g1 \
			--payload '{"statement":"ship stage five"}')"
		case "$out" in *ticked=true*) ;; *) echo "goal observe: $out"; exit 1 ;; esac
		out="$("$MH" control pull --addr "$addr" --principal "$principal" --token-file "$tok")"
		post="${out##*digest=}"; post="${post%% *}"
		[ -n "$pre" ] && [ -n "$post" ] && [ "$pre" != "$post" ] || { echo "goal write did not change the scoped digest (pre=$pre post=$post)"; exit 1; }

		# Governed pull CONTENT leg: the goal statement itself arrives via the pull verb
		# (control pull --json emits the scoped projection's resources + fields).
		"$MH" control pull --json --addr "$addr" --principal "$principal" --token-file "$tok" \
			| grep -q "ship stage five" || { echo "goal content did not arrive via the governed pull verb"; exit 1; }

		{ kill "$runpid" 2>/dev/null; wait "$runpid"; } 2>/dev/null || true
		rm -f "$PIDFILE"
		sleep 0.3

		# NEGATIVE: a malformed second package must REFUSE boot (directory presence is contract;
		# split streams — the "ready" banner goes to stdout, the refusal names the path on stderr).
		# Background launch + bounded poll: a foreground run would HANG the suite if fail-closed
		# ever regressed into a serving process, so the refusal must arrive within ~6s or the
		# process is killed and the leg fails.
		mkdir -p .mnemon/loops/bad
		printf '{nope' >.mnemon/loops/bad/capability.json
		"$MH" local run >"$WORK/external-bad.out.log" 2>"$WORK/external-bad.err.log" &
		local badpid=$! refused=0
		for i in $(seq 1 60); do
			if ! kill -0 "$badpid" 2>/dev/null; then
				refused=1
				break
			fi
			sleep 0.1
		done
		if [ "$refused" != 1 ]; then
			kill "$badpid" 2>/dev/null || true
			wait "$badpid" 2>/dev/null || true
			echo "local run still alive after 6s with a malformed external package (fail-closed regressed)"; exit 1
		fi
		if wait "$badpid"; then
			echo "local run must exit non-zero with a malformed external package"; exit 1
		fi
		grep -q ".mnemon/loops/bad" "$WORK/external-bad.err.log" || { echo "boot refusal must name the bad package path on stderr"; cat "$WORK/external-bad.err.log"; exit 1; }
		rm -rf .mnemon/loops/bad
	) || fail "external goal flow failed (see $WORK/run-external-goal.log)"
	sleep 0.3
	echo "    external goal package OK"
}

# Both hosts run sequentially (the server is stopped between them). codex stays on the default
# port (covering the bare default path); claude-code deliberately runs on a NON-default port to
# pin the stage-0 promise that a bare `local run` listens where setup's --control-url pointed.

# run_sync_pair proves the stage-6 Remote MVP on the product path: two replicas (A, B) sync
# through a standalone mnemond hub over TLS — A writes, the in-process sync worker pushes, B's
# worker pulls and the content arrives via B's governed pull (attribution carried end to end).
# Offline leg pins I13 (hub down = local fully functional); the bad-token leg pins authn on the
# wire. Conflict adjudication (hub idempotency + B-side import conflict) is pinned at the Go
# integration layer (syncserver_test.go, sync_import_test.go) per the v1.1 redefinition.
run_sync_pair() {
	CUR_HOST="sync-pair"
	echo "=== E2E sync pair via mnemond (TLS) ==="
	local hubdir="$WORK/hub" tlsdir="$WORK/synctls"
	mkdir -p "$hubdir" "$tlsdir"

	go build -o "$WORK/mnemond" ./harness/cmd/mnemond

	"$WORK/mnemond" --dev-selfsigned "$tlsdir" >/dev/null
	[ -f "$tlsdir/cert.pem" ] && [ -f "$tlsdir/key.pem" ] || fail "dev-selfsigned did not write cert/key"

	# hub credentials: two replicas, distinct principals (multi-replica acceptance).
	printf '%s\n' "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666" >"$hubdir/replica-a.token"
	printf '%s\n' "9999aaaa8888bbbb7777cccc6666dddd5555eeee4444ffff" >"$hubdir/replica-b.token"
	chmod 600 "$hubdir/replica-a.token" "$hubdir/replica-b.token"
	cat >"$hubdir/replicas.json" <<-'JSON'
	{
	  "schema_version": 1,
	  "replicas": [
	    {"principal": "replica-a@hub", "credential_ref": "replica-a.token",
	     "scopes": [{"kind": "memory", "id": "project"}, {"kind": "skill", "id": "project"}]},
	    {"principal": "replica-b@hub", "credential_ref": "replica-b.token",
	     "scopes": [{"kind": "memory", "id": "project"}]}
	  ]
	}
	JSON
	chmod 600 "$hubdir/replicas.json"

	"$WORK/mnemond" --addr 127.0.0.1:9787 --store "$hubdir/hub.db" --replicas "$hubdir/replicas.json" \
		--tls-cert "$tlsdir/cert.pem" --tls-key "$tlsdir/key.pem" >"$WORK/mnemond.log" 2>&1 &
	local hubpid=$!
	sleep 0.5
	kill -0 "$hubpid" 2>/dev/null || { cat "$WORK/mnemond.log"; fail "mnemond did not start"; }

	local proja="$WORK/proj-sync-a" projb="$WORK/proj-sync-b"
	mkdir -p "$proja" "$projb"
	local apid="" bpid=""
	(
		cd "$proja"
		local tok=".mnemon/harness/channel/credentials/codex-project.token"
		"$MH" setup --host codex --memory --principal codex@project --control-url http://127.0.0.1:8787 >/dev/null
		"$MH" sync connect hub --remote-url https://127.0.0.1:9787 \
			--token-file "$hubdir/replica-a.token" --ca-file "$tlsdir/cert.pem" >/dev/null
		"$MH" local run --sync-interval 100ms >"$WORK/run-sync-a.log" 2>&1 &
		echo $! >"$WORK/sync-a.pid"
		local up=0 i
		for i in $(seq 1 60); do
			"$MH" control status --addr http://127.0.0.1:8787 --principal codex@project --token-file "$tok" >/dev/null 2>&1 && { up=1; break; }
			sleep 0.1
		done
		[ "$up" = 1 ] || { cat "$WORK/run-sync-a.log"; exit 1; }
		"$MH" control observe --addr http://127.0.0.1:8787 --principal codex@project --token-file "$tok" \
			--type memory.write_candidate.observed --external-id sp1 \
			--payload '{"content":"sync pair payload from replica A","source":"user","confidence":"high"}' >/dev/null
	) || fail "replica A flow failed (see $WORK/run-sync-a.log / $WORK/mnemond.log)"
	apid="$(cat "$WORK/sync-a.pid")"

	(
		cd "$projb"
		local tok=".mnemon/harness/channel/credentials/codex-project.token"
		"$MH" setup --host codex --memory --principal codex@project --control-url http://127.0.0.1:8899 >/dev/null
		"$MH" sync connect hub --remote-url https://127.0.0.1:9787 \
			--token-file "$hubdir/replica-b.token" --ca-file "$tlsdir/cert.pem" >/dev/null
		"$MH" local run --sync-interval 100ms >"$WORK/run-sync-b.log" 2>&1 &
		echo $! >"$WORK/sync-b.pid"
		local up=0 i seen=0
		for i in $(seq 1 60); do
			"$MH" control status --addr http://127.0.0.1:8899 --principal codex@project --token-file "$tok" >/dev/null 2>&1 && { up=1; break; }
			sleep 0.1
		done
		[ "$up" = 1 ] || { cat "$WORK/run-sync-b.log"; exit 1; }
		# A worker pushes -> hub -> B worker pulls -> import re-enters intake -> governed pull sees it.
		for i in $(seq 1 100); do
			if "$MH" control pull --json --addr http://127.0.0.1:8899 --principal codex@project --token-file "$tok" 2>/dev/null | grep -q "sync pair payload from replica A"; then
				seen=1
				break
			fi
			sleep 0.2
		done
		# Diagnosable-flake margin (LOW-11): assert the hub actually RECEIVED A's push, separately
		# from B's pull arriving. A flake then reads as "push never arrived" (received=0) vs "pull
		# never ran" (received>=1 but B unseen) instead of one opaque timeout. /sync/status accepts
		# GET (frozen verb-method map); replica-a's token authorizes it over the pinned TLS cert.
		local hubstatus
		hubstatus="$(curl -sS --cacert "$tlsdir/cert.pem" \
			-H "Authorization: Bearer $(tr -d '\n' <"$hubdir/replica-a.token")" \
			https://127.0.0.1:9787/sync/status 2>/dev/null)"
		case "$hubstatus" in
			*'"hub_commits_received":0'*|'') echo "hub never received A's push (status: ${hubstatus:-<empty>})"; tail -5 "$WORK/run-sync-b.log"; exit 1 ;;
			*'"hub_commits_received":'*) ;;
			*) echo "unexpected hub status: $hubstatus"; exit 1 ;;
		esac
		[ "$seen" = 1 ] || { echo "B never saw A's commit within 20s (hub received the push: $hubstatus -> pull side failed)"; tail -5 "$WORK/run-sync-b.log"; exit 1; }
		# attribution: the import preserves A's entries VERBATIM (faithful provenance) and the
		# write itself is attributed to the sync importer; the full origin chain (replica id,
		# decision id) lives in B's event log + decisions, pinned by sync_import Go tests.
		"$MH" control pull --json --addr http://127.0.0.1:8899 --principal codex@project --token-file "$tok" | grep -q '"sync@local"' \
			|| { echo "imported resource lacks sync@local attribution"; exit 1; }
	) || fail "replica B flow failed (see $WORK/run-sync-b.log / $WORK/mnemond.log)"
	bpid="$(cat "$WORK/sync-b.pid")"

	# offline leg (I13): hub down, A stays fully functional on the product path.
	kill "$hubpid" 2>/dev/null; wait "$hubpid" 2>/dev/null || true
	(
		cd "$proja"
		local tok=".mnemon/harness/channel/credentials/codex-project.token"
		out="$("$MH" control observe --addr http://127.0.0.1:8787 --principal codex@project --token-file "$tok" \
			--type memory.write_candidate.observed --external-id sp-offline \
			--payload '{"content":"offline write while hub is down","source":"user","confidence":"high"}')"
		case "$out" in *ticked=true*) ;; *) echo "offline observe: $out"; exit 1 ;; esac
		"$MH" control pull --addr http://127.0.0.1:8787 --principal codex@project --token-file "$tok" >/dev/null
	) || fail "I13 offline leg failed"

	# authn leg: with A's server stopped (lock released), a manual push with the WRONG token is refused.
	{ kill "$apid" 2>/dev/null; wait "$apid"; } 2>/dev/null || true
	{ kill "$bpid" 2>/dev/null; wait "$bpid"; } 2>/dev/null || true
	"$WORK/mnemond" --addr 127.0.0.1:9787 --store "$hubdir/hub.db" --replicas "$hubdir/replicas.json" \
		--tls-cert "$tlsdir/cert.pem" --tls-key "$tlsdir/key.pem" >>"$WORK/mnemond.log" 2>&1 &
	hubpid=$!
	sleep 0.5
	(
		cd "$proja"
		# sp-offline is still pending (hub was down when it was written), so the push really
		# sends a request. The stored credential is what the client uses - corrupt it for the
		# negative, restore for the positive (true product-path authn probe).
		# connect stored the absolute --token-file path as credential_ref; mnemond loaded the
		# token into memory at boot, so editing the file flips only the CLIENT side.
		local cred="$hubdir/replica-a.token"
		cp "$cred" "$WORK/cred.bak"
		printf '%s\n' "000000000000000000000000000000000000000000000000" >"$cred"
		if "$MH" sync push --once >/dev/null 2>&1; then
			echo "unknown-token push must be refused"; exit 1
		fi
		cp "$WORK/cred.bak" "$cred"
		"$MH" sync push --once >/dev/null 2>&1 || { echo "right-token push must succeed"; exit 1; }
	) || fail "authn leg failed"
	kill "$hubpid" 2>/dev/null; wait "$hubpid" 2>/dev/null || true
	rm -f "$PIDFILE"
	echo "    sync pair via mnemond OK"
}

run_host codex codex@project 8787 .codex
run_host claude-code claude@project 8899 .claude
run_skill codex codex@project
run_skill claude-code claude@project
run_note
run_external_goal
run_sync_pair

echo "E2E PASS (codex + claude-code; memory + skill + note-via-config + external-goal + sync-pair)"
