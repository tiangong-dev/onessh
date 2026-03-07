# onessh

[中文](README.zh-CN.md)

OneSSH is a CLI SSH manager built around a single master password. All host addresses, credentials, and configuration are encrypted at rest — you unlock everything once, then connect, run commands, and transfer files without ever typing credentials again.

The data directory is safe to push to a **public** Git repository — all sensitive fields are encrypted with AES-256-GCM and the key is derived via Argon2id. Only the `ENC[...]` ciphertext is stored on disk; the master password never touches the filesystem. Use a **strong master password** (12+ characters, mixed case, digits, and symbols) to ensure brute-force resistance.

## Core — Encrypted Credential Management

- `onessh init` initialize encrypted config
- `onessh passwd` change master password
- `onessh add <alias>` add a host (interactive or with flags)
- `onessh update <alias>` update a host
- `onessh rm <alias>` remove a host
- `onessh ls [--tag <tag>] [--filter <glob>]` list hosts with summary; filter by tag or glob pattern
- `onessh show <alias>` show detailed information for a host
- `onessh user ls / add / update / rm` manage reusable user profiles
- `onessh logout [--all]` clear cached master password (or all cached master passwords)
- `onessh agent start|stop|status|clear-all` manage in-memory cache agent
- Hosts reference reusable user profiles via `user_ref`; auth lives at the profile level
- Host-level `env`, `pre_connect` / `post_connect` hooks, `tags`
- Master password cached for 10 minutes by default

## Also — SSH Operations

- `onessh <alias> [-- <ssh-args...>]` connect interactively (SSH argument passthrough supported)
- `onessh exec <alias> <command> [args...]` run a command non-interactively; stdout/stderr piped through
- `onessh exec --tag <tag> <command>` batch exec on hosts matching tag
- `onessh cp <src>... <dst>` copy files via scp using `alias:path` notation; supports multi-file upload and remote-to-remote
- `onessh cp --tag <tag> <files>... :/path` batch upload to hosts matching tag
- `onessh test [<alias>]` check SSH connectivity; `--all`, `--tag`, `--filter` for batch testing
- `onessh completion bash|zsh|fish|powershell` shell completion (tab-completes host aliases)
- `onessh version` print version/build info

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

## Quick Start

```bash
onessh init
onessh add web1
onessh ls
onessh web1
onessh web1 -- -L 8080:127.0.0.1:80 -N
```

When adding a host, you can create a new user profile or select an existing one.

## Shell Completion

```bash
# zsh
onessh completion zsh > "${fpath[1]}/_onessh"

# bash
onessh completion bash > /etc/bash_completion.d/onessh

# fish
onessh completion fish > ~/.config/fish/completions/onessh.fish
```

Once enabled, `onessh <Tab>` completes host aliases using the agent cache (no password prompt).

## Host Operations

### Connect

```bash
onessh web1
onessh web1 -- -L 8080:127.0.0.1:80 -N
```

### Run a remote command

```bash
onessh exec web1 uptime
onessh exec web1 df -h /
onessh exec web1 -- bash -c "cd /srv && ls"
```

### Copy files

```bash
onessh cp web1:/etc/hosts ./hosts          # download
onessh cp ./deploy.sh web1:/tmp/           # upload
onessh cp file1 file2 web1:/tmp/           # multi-file upload
onessh cp -r web1:/var/log/app ./logs      # recursive download
onessh cp web1:/etc/hosts web2:/tmp/       # remote-to-remote
```

### Test connectivity

```bash
onessh test web1
onessh test --all
onessh test --all --timeout 3
```

### Show host details

```bash
onessh show web1
```

## Batch Operations

Commands that operate on remote hosts support batch execution via `--all`, `--tag`, and `--filter`.

### `--filter` glob pattern

`--filter` accepts a glob pattern (Go `filepath.Match` syntax) that matches against host alias, host address, or description (OR logic — match any).

Supported wildcards:

- `*` matches any sequence of characters
- `?` matches a single character
- `[abc]` matches one character in the set
- `[a-z]` matches one character in the range

Note: this is **full-string matching**, not substring. Use `*substr*` for substring matching.

### Examples

```bash
# exec on multiple hosts
onessh exec --all uptime
onessh exec --tag prod uptime
onessh exec --filter "web*" -- df -h /
onessh exec --tag prod --filter "cn-*" uptime    # tag AND filter combined

# test connectivity
onessh test --all
onessh test --tag prod
onessh test --filter "192.168.*"

# batch upload
onessh cp --tag prod deploy.sh :/tmp/
onessh cp --filter "web*" app.conf :/etc/app/
onessh cp --tag prod -r dist/ :/srv/app/
```

### Dry run

Add `--dry-run` to preview matched hosts without executing the operation:

```bash
onessh exec --tag prod --dry-run uptime
onessh cp --filter "web*" --dry-run app.conf :/etc/app/
onessh test --all --dry-run
```

## Host Management

### Add and tag hosts

```bash
onessh add web1 --tag prod --tag cn
onessh add staging --tag staging
```

### Update

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
onessh update ais --tag prod --untag staging
onessh update ais --clear-tags
```

### List and filter

```bash
onessh ls
onessh ls --tag prod
onessh ls --filter "web*"
onessh ls --tag prod --filter "cn-*"
```

### Hook behavior

- `pre_connect` runs first, then an interactive shell starts, then `post_connect` runs after the shell exits.
- To jump directly into a root shell: `--pre-connect "exec sudo su -"`.

## Configuration

Default data path:

```text
~/.config/onessh/data
```

Override options:

- Environment variable: `ONESSH_DATA`
- CLI flag: `--data /path/to/data`
- CLI flag: `--cache-ttl 10m` (default: 10 minutes)
- CLI flag: `--no-cache` to disable cache
- CLI flag: `--agent-socket /path/to/agent.sock`
- CLI flag: `--agent-capability <token>` to require capability-auth for agent IPC
- Environment variable: `ONESSH_AGENT_SOCKET` (fallback: `SHUSH_SOCKET`)
- Environment variable: `ONESSH_AGENT_CAPABILITY` (fallback: `SHUSH_CAPABILITY`)

Memory backend behavior:

- Master password cache is memory-agent only (no file cache).
- Agent auto-starts on first successful password entry.
- Manage manually via `onessh agent start|status|stop|clear-all`.
- Use `onessh logout --all` to wipe all onessh master-password cache entries.
- When capability is configured, all agent requests (including askpass token flow) must carry the same token.
- If capability is not configured, `onessh agent start` generates a random session token and prints an export command (not persisted).
- To export directly into current shell, use: `eval "$(onessh agent start --print-env)"`.

Password auth note:

- Password auth first tries `sshpass -d` (FD-based, no secret env var).
- If `sshpass` is unavailable, falls back to `SSH_ASKPASS` + onessh agent IPC token (short-lived and low-use by default).

## Store Layout

```text
~/.config/onessh/data/
  meta.yaml
  users/
    <alias>.yaml
  hosts/
    <alias>.yaml
```

Sensitive field values are stored as `ENC[...]`; the file structure stays diff-friendly.

```yaml
# ~/.config/onessh/data/users/ops.yaml
version: 1
name: ENC[v1,...]
auth:
  type: key
  key_path: ENC[v1,...]
```

```yaml
# ~/.config/onessh/data/hosts/ais.yaml
version: 1
host: ENC[v1,...]
user_ref: ops
port: 22
tags:
  - prod
env:
  AWS_PROFILE: ENC[v1,...]
  HTTPS_PROXY: ENC[v1,...]
pre_connect:
  - ENC[v1,...]
post_connect:
  - ENC[v1,...]
```

## Security Notes

- Encryption: Argon2id + AES-256-GCM
- Only encrypted data is stored on disk (Git-friendly)
- Master password and plaintext only exist in memory at runtime
- Detailed design and flowcharts: [`docs/security.md`](docs/security.md)

## Automated Release (GitHub Actions)

This repository includes a `release` workflow:

- Trigger: push tag `v*` (e.g. `v0.2.0`)
- Actions:
  - Build multi-platform binaries (Linux/macOS/Windows, amd64/arm64)
  - Create GitHub Release and checksums automatically
  - Update Homebrew formula (`Formula/onessh.rb`) automatically

Release example:

```bash
git tag v0.2.0
git push origin v0.2.0
```

Before first release, ensure `Actions > Workflow permissions` is set to **Read and write permissions**.
