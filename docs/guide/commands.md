# Commands

## Credential and store management

| Command | Description |
| --- | --- |
| `onessh init` | Initialize encrypted config |
| `onessh passwd` | Change master password |
| `onessh add <alias>` | Add a host |
| `onessh update <alias>` | Update a host |
| `onessh rm <alias>` | Remove a host |
| `onessh ls [--tag] [--filter]` | List hosts |
| `onessh show <alias>` | Show host details |
| `onessh user` (`ls`, `add`, `update`, `rm`) | Manage user profiles |
| `onessh logout [--all]` | Clear cached master password |
| `onessh log` | Show recent audit entries (`--last`, `--action`, `--format`); subcommands `enable` / `disable` / `status` |
| `onessh agent` (`start`, `stop`, `status`, `clear-all`) | Memory cache agent |

## SSH operations

| Command | Description |
| --- | --- |
| `onessh <alias> [-- ssh-args...]` | Interactive SSH |
| `onessh exec <alias> <cmd> [args...]` | Non-interactive remote command |
| `onessh exec --tag <tag> <cmd>` | Batch exec by tag |
| `onessh cp <src>... <dst>` | Copy via scp (`alias:path` notation) |
| `onessh cp --tag <tag> files... :/path` | Batch upload by tag |
| `onessh test [<alias>]` | Connectivity check; `--all`, `--tag`, `--filter` |
| `onessh completion` (`bash`, `zsh`, `fish`, `powershell`) | Shell completion |
| `onessh version` | Version and build info |

## Batch selectors

Remote commands support `--all`, `--tag <tag>`, and `--filter <glob>` (Go `filepath.Match`, full-string match). Combine tag and filter with AND semantics where supported. Use `--dry-run` to list matched hosts without running the operation.

See the [repository README](https://github.com/tiangong-dev/onessh/blob/main/README.md) for extended examples.
