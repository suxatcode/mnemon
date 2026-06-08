#!/usr/bin/env bash
set -euo pipefail
# Validate the embedded harness loop/host/binding manifests via the harness binary.
go run ./harness/cmd/mnemon-harness loop validate "$@"
