# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Mnemon, please report it responsibly:

1. **Do NOT open a public GitHub issue.**
2. Use [GitHub Security Advisories](https://github.com/suxatcode/mnemon/security/advisories/new) to report privately.
3. Include steps to reproduce, affected versions, and potential impact.

We will acknowledge receipt within 48 hours and aim to release a fix within 7 days for critical issues.

## Scope

Mnemon runs locally and stores data in `~/.mnemon/`. Key security considerations:

- **SQLite database** — contains all stored insights; protected by filesystem permissions (`0644`).
- **Hook scripts** — shell scripts executed by the LLM CLI at lifecycle events; written with `0755` permissions.
- **Ollama connection** — optional HTTP calls to a local Ollama instance; no TLS by default. If `MNEMON_EMBED_ENDPOINT` is pointed at a remote server, traffic is unencrypted unless the endpoint uses HTTPS.

## Supported Versions

| Version | Supported |
|---|---|
| Latest release | Yes |
| Older releases | Best effort |
