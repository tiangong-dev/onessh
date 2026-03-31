# Getting started

OneSSH is a CLI that stores SSH hosts and credentials encrypted under one master password. After you unlock once (cached in memory), routine SSH work does not prompt for passwords again.

## Install

### Homebrew (macOS / Linux)

```bash
brew tap tiangong-dev/onessh https://github.com/tiangong-dev/onessh
brew install tiangong-dev/onessh/onessh
```

### Build from source

```bash
git clone https://github.com/tiangong-dev/onessh.git
cd onessh
make build
```

The `onessh` binary is written to the repository root.

## Initialize

```bash
onessh init
```

Choose a **strong master password** (12+ characters with mixed case, digits, and symbols). It is never written to disk; only derived key material and ciphertext are stored.

## Add a host

Interactive:

```bash
onessh add web1
```

You can create a new user profile or attach an existing one via `user_ref`.

## Connect

```bash
onessh web1
```

Pass extra SSH flags after `--`:

```bash
onessh web1 -- -L 8080:127.0.0.1:80 -N
```

## Next steps

- [Commands](./commands.md) — full command reference overview
- [Configuration](./configuration.md) — data path, agent, environment variables
- [Architecture](../architecture.md) — design and execution flow
- [Security](../security.md) — threat model and mitigations
