# Configuration

## Data directory

Default:

```text
~/.config/onessh/data
```

Overrides:

- Environment: `ONESSH_DATA`
- Flag: `--data /path/to/data`

## Cache and agent

- `--cache-ttl 10m` — master password cache lifetime (default 10 minutes)
- `--no-cache` — disable cache
- `--agent-socket /path/to/agent.sock` — agent Unix socket
- `--agent-capability <token>` — require capability token for agent IPC

Environment variables for defaults (when flags are unset):

- `ONESSH_AGENT_SOCKET`
- `ONESSH_AGENT_CAPABILITY`

The agent stores the master password in memory only. It can auto-start on first successful unlock; when not explicitly configured, socket and capability default from your parent shell PID for convenience and namespace separation between terminals, not as a strong same-UID security boundary.

## Store layout

```text
~/.config/onessh/data/
  meta.yaml
  users/
    <alias>.yaml
  hosts/
    <alias>.yaml
```

Sensitive values are `ENC[...]` ciphertext. Structure stays readable for Git diffs.

## Host entry fields (YAML)

Each host can include:

- **`proxy_jump`** — passed to `ssh` / `scp` as `-J` (jump host).
- **`env`** — per-host environment variables merged into the SSH process; keys are also sent to the server via `SendEnv` when the remote `sshd` allows it.
- **`pre_connect` / `post_connect`** — local hook commands run inside a remote login shell wrapper before/after the interactive session. They are incompatible with SSH `-N` and `-T` (OneSSH rejects that combination).

Use `onessh add` / `onessh update` to edit these; see [Commands](/guide/commands) and [Architecture](/reference/architecture) for the full model.

## Password authentication

- Prefer `sshpass -d` when available (file descriptor, not environment).
- Otherwise: `SSH_ASKPASS` with a short-lived single-use onessh agent token. This fallback is weaker than `sshpass -d` because the helper still needs a short-lived bearer token to resolve the password at runtime.

For encryption details and runtime security, see [Security](/reference/security).
