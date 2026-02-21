# Contributing to Mnemon

Thank you for considering contributing to Mnemon!

## Development Setup

```bash
git clone https://github.com/mnemon-dev/mnemon.git
cd mnemon
make build
```

## Running Tests

```bash
make test
```

This runs the full E2E test suite (`scripts/e2e_test.sh`).

## Submitting Changes

1. Fork the repository and create a feature branch from `master`.
2. Make your changes and ensure `make test` passes.
3. Open a pull request against `master`.

## Releasing

Releases are fully automated. Maintainers tag and push:

```bash
git tag v0.2.0
git push origin v0.2.0
```

This triggers GitHub Actions → runs tests → builds cross-platform binaries via GoReleaser → publishes a GitHub Release → updates the Homebrew tap.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
