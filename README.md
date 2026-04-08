# onessh

[中文](README.zh-CN.md)

**Documentation:** [https://tiangong-dev.github.io/onessh/](https://tiangong-dev.github.io/onessh/) (English · [简体中文](https://tiangong-dev.github.io/onessh/zh/))

OneSSH is a CLI SSH manager built around a single master password. Hosts, credentials, and config are encrypted at rest (Argon2id + AES-256-GCM); you unlock once, then connect, run commands, and copy files without retyping secrets. The data directory is safe to push to a **public** Git repository — only `ENC[...]` ciphertext is stored; use a **strong master password** (12+ characters, mixed case, digits, symbols).

**Full user guide:** command tables, configuration, architecture, and security are covered in the **[documentation site](https://tiangong-dev.github.io/onessh/)**. This README keeps build/install/release and a minimal quick start; for long examples (batch ops, hooks, store layout), see the docs or the historical sections still in [README.zh-CN](README.zh-CN.md) if needed.

## Quick start

Minimal flow after install: create the encrypted store, register a host, list entries, then open a session. Replace `web1` with your own host alias.

```bash
# Create ~/.onessh (or $ONESSH_HOME) and set the master password used to encrypt secrets
onessh init
# Interactive wizard: host, user, auth (key/password), optional port and labels
onessh add web1
# Show saved host aliases and metadata (passwords stay encrypted until unlock)
onessh ls
# Unlock if needed, then SSH to the host named web1
onessh web1
```

## Build

```bash
make build
```

Release-style build (version ldflags):

```bash
make build-release VERSION=v0.0.0
```

## Test

```bash
make test          # includes e2e
make test-short    # skips e2e
make test-e2e      # e2e only
```

## Install (Homebrew)

```bash
brew tap tiangong-dev/onessh https://github.com/tiangong-dev/onessh
brew install tiangong-dev/onessh/onessh
```

## Design references (source)

- [Architecture](docs/reference/architecture.md) — modules and execution flow  
- [Security](docs/reference/security.md) — threat model and mitigations  

## Automated release

Push a tag `v*` (e.g. `v0.2.0`) to build binaries, create a GitHub Release, and update the Homebrew formula. Before the first release, set **Actions → Workflow permissions** to **Read and write**.

```bash
git tag v0.2.0
git push origin v0.2.0
```
