# onessh

[中文](README.zh-CN.md)

OneSSH is a Go-based SSH host manager that encrypts configuration with a single master password.

## Features

- `onessh init` initialize encrypted config
- `onessh add <alias>` add a host
- `onessh update <alias>` update a host (interactive or with generic flags)
- `onessh rm <alias>` remove a host
- `onessh ls` list hosts with user/auth/port summary
- `onessh user ls` list reusable users
- `onessh user add <alias> --name <user>` add a reusable user (with auth)
- `onessh user update <alias>` update reusable user auth/profile
- `onessh user rm <alias>` remove a reusable user
- `onessh logout` clear cached master password
- `onessh agent start|stop|status` manage in-memory cache agent
- `onessh version` print version/build info
- `onessh dump` print decrypted YAML to stdout (redacted by default; use `--show-secrets` to reveal)
- `onessh sshconfig export|import` sync with `~/.ssh/config`
- `onessh <alias> [-- <ssh-args...>]` or `onessh connect <alias> [-- <ssh-args...>]` connect via SSH (supports SSH argument passthrough)
- Hosts reference user profiles via `user_ref`
- Auth is maintained at profile level
- Host-level `env` is applied to local SSH process and forwarded with `SendEnv`
- `pre_connect` / `post_connect` hooks for per-host remote bootstrap/cleanup commands
- Master password cache: no re-prompt within 10 minutes by default

## Build

```bash
go build -o onessh ./cmd/onessh
```

## Install (Homebrew)

The release pipeline automatically updates `Formula/onessh.rb`.

```bash
brew tap tiangong-dev/onessh https://github.com/tiangong-dev/onessh
brew install tiangong-dev/onessh/onessh
```

Upgrade:

```bash
brew update
brew upgrade onessh
```

## Configuration

Default path:

```text
~/.config/onessh/config
```

Override options:

- Environment variable: `ONESSH_CONFIG`
- CLI flag: `--config /path/to/config`
- CLI flag: `--cache-ttl 10m` (default: 10 minutes)
- CLI flag: `--no-cache` to disable cache
- CLI flag: `--agent-socket /path/to/agent.sock` (for memory backend)
- Environment variable: `ONESSH_AGENT_SOCKET` to customize memory agent socket path

Memory backend behavior:

- Master password cache is memory-agent only (no file cache compatibility).
- Agent auto-starts on first successful password entry.
- You can manage it manually via `onessh agent start|status|stop`.

Password auth note:

- Password auth first tries `sshpass -d` (FD-based, no secret env var).
- If `sshpass` is unavailable, it falls back to `SSH_ASKPASS` + onessh agent IPC token (short-lived).

Store layout (sharded + SOPS-like encrypted values):

```text
~/.config/onessh/config/
  meta.yaml
  users/
    <alias>.yaml
  hosts/
    <alias>.yaml
```

Sensitive field values are stored as `ENC[...]` while the file structure remains diff-friendly.

Example files:

```yaml
# ~/.config/onessh/config/users/ops.yaml
version: 1
name: ENC[v1,...]
auth:
  type: key
  key_path: ENC[v1,...]
```

```yaml
# ~/.config/onessh/config/hosts/ais.yaml
version: 1
host: ENC[v1,...]
user_ref: ops
port: 22
env:
  AWS_PROFILE: ENC[v1,...]
  HTTPS_PROXY: ENC[v1,...]
pre_connect:
  - ENC[v1,...]
post_connect:
  - ENC[v1,...]
```

## Quick Start

```bash
./onessh init
./onessh add web1
./onessh ls
./onessh web1
./onessh web1 -- -L 8080:127.0.0.1:80 -N
```

When adding a host, you can:

- create a new user profile by entering a username, or
- select an existing user profile from the list.

User profile YAML shape:

```yaml
users:
  ops:
    name: ubuntu
    auth:
      type: key
      key_path: ~/.ssh/id_ed25519
```

Host entries must include `user_ref` and do not keep `auth` / `user` fields at host level.

Non-interactive host update examples:

```bash
onessh update ais --alias pi
onessh update ais --host 10.0.0.12 --port 2222
onessh update ais --user-ref ops
onessh update ais --user ubuntu --auth-type key --key-path ~/.ssh/id_ed25519
onessh update ais --env AWS_PROFILE=prod --env HTTPS_PROXY=http://127.0.0.1:7890
onessh update ais --unset-env HTTPS_PROXY
onessh update ais --clear-env
onessh update ais --pre-connect "cd /srv/app" --pre-connect "source .envrc"
onessh update ais --post-connect "echo disconnected"
onessh update ais --clear-pre-connect --clear-post-connect
```

Hook behavior notes:

- `pre_connect` runs first, then an interactive shell starts, then `post_connect` runs after the shell exits.
- To jump directly into root shell, use `--pre-connect "exec sudo su -"`.

SSH config interop:

```bash
onessh sshconfig export
onessh sshconfig export --stdout
onessh sshconfig import
onessh sshconfig import --overwrite
```

- `export` writes a managed block into `~/.ssh/config`.
- `import` reads compatible `Host` entries (wildcards are ignored).

## Security Notes

- Encryption: Argon2id + AES-256-GCM
- Only encrypted data is stored on disk (Git-friendly)
- Master password and plaintext only exist in memory at runtime
- Detailed design and flowcharts: [`docs/security.md`](docs/security.md)

## Automated Release (GitHub Actions)

This repository includes a `release` workflow:

- Trigger: push tag `v*` (for example `v0.2.0`)
- Actions:
  - Build multi-platform binaries (Linux/macOS/Windows, amd64/arm64)
  - Create GitHub Release and checksums automatically
  - Update Homebrew formula (`Formula/onessh.rb`) automatically

Release example:

```bash
git tag v0.2.0
git push origin v0.2.0
```

Before first release, ensure repository setting `Actions > Workflow permissions` is set to **Read and write permissions** so formula updates can be pushed.
