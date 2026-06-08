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
			--type memory.write_candidate_observed --external-id m1 \
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
			--type memory.write_candidate_observed --external-id bad1 \
			--payload '{"content":"api_key=sk-abcdefABCDEF123456","source":"user","confidence":"high"}' >/dev/null
		out="$("$MH" control pull --addr "$addr" --principal "$principal" --token-file "$tok")"
		case "$out" in *resources=1*) ;; *) echo "negative pull leaked: $out"; exit 1 ;; esac

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

# Both hosts run sequentially (the server is stopped between them), so they share the default
# local-run bind addr; the port is the same for both.
run_host codex codex@project 8787 .codex
run_host claude-code claude@project 8787 .claude

echo "E2E PASS (codex + claude-code)"
