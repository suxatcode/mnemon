# Mnemon Harness

`mnemon-harness` is an experimental Agent Integration layer for connecting host
agents to Local Mnemon.

The current product surface is intentionally small:

- `setup` installs memory/skill integration assets into Codex or Claude Code.
- `local run` starts the project-local Mnemon service.
- `status` reports Agent Integration, Local Mnemon, and sync status.
- `sync` connects Local Mnemon to a Remote Workspace (`mnemon-hub`) and pushes/pulls
  governed commits with attribution preserved.
- `loop validate` remains hidden and is used by `make harness-validate`.

Host directories such as `.codex` and `.claude` are projection surfaces. Runtime
state is under `.mnemon/harness/`, and release-path Mnemon behavior stays under
`cmd/` and `internal/`.

## Build

From the repository root:

```sh
go build -o mnemon .
go build -o mnemon-harness ./harness/cmd/mnemon-harness
```

Validate harness declarations:

```sh
make harness-validate
```

## Try The Harness

Install memory and skill integration for a host:

```sh
./mnemon-harness setup --host codex --memory --skills --project-root .
./mnemon-harness local run
./mnemon-harness status
```

Remove projected assets for a principal:

```sh
./mnemon-harness setup uninstall --host codex --memory --skills --principal codex@project --project-root .
```

More command examples are in `docs/harness/USAGE.md`.
