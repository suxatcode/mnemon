# Changelog

All notable changes to Mnemon will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Release pipeline: GoReleaser, GitHub Actions, Homebrew tap
- `--version` flag
- CONTRIBUTING.md, CHANGELOG.md

## [0.1.0] - TBD

Initial public release.

### Added
- Core CRUD: `remember`, `recall`, `forget`, `search`, `status`, `log`
- Four-graph architecture: temporal, entity, causal, semantic edges
- Intent-aware smart recall with beam search graph traversal
- Built-in deduplication and conflict resolution
- Retention lifecycle: importance decay, access-count boosting, garbage collection
- Optional embedding support via Ollama (`nomic-embed-text`)
- Knowledge graph visualization (`mnemon viz`)
- Claude Code integration via hooks (prime, remind, nudge, compact)
- OpenClaw integration via skill deployment
- `mnemon setup` interactive installer with `--eject` support
- Comprehensive documentation with Chinese translations

[Unreleased]: https://github.com/mnemon-dev/mnemon/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/mnemon-dev/mnemon/releases/tag/v0.1.0
