# onessh

English | [中文](#中文说明)

OneSSH is a Go-based SSH host manager that encrypts the entire configuration with a single master password.

## Features

- `onessh init` initialize encrypted config
- `onessh add <alias>` add a host
- `onessh update <alias>` update a host (interactive or with generic flags)
- `onessh rm <alias>` remove a host
- `onessh ls` list host aliases
- `onessh user ls` list reusable users
- `onessh user add <alias> --name <user>` add a reusable user (with auth)
- `onessh user update <alias>` update reusable user auth/profile
- `onessh user rm <alias>` remove a reusable user
- `onessh logout` clear cached master password
- `onessh version` print version/build info
- `onessh dump` print decrypted YAML to stdout
- `onessh <alias>` or `onessh connect <alias>` connect via SSH
- Reusable user profiles: hosts can reference shared users (`user_ref`)
- Auth is profile-level: user profiles include auth settings
- Master password cache: by default, no re-prompt within 10 minutes

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

## Configuration File

Default path:

```text
~/.onessh/config.enc
```

Override options:

- Environment variable: `ONESSH_CONFIG`
- CLI flag: `--config /path/to/config.enc`
- CLI flag: `--cache-ttl 10m` (default: 10 minutes)
- CLI flag: `--no-cache` to disable cache
- Environment variable: `ONESSH_CACHE_FILE` to customize cache file path

## Quick Start

```bash
./onessh init
./onessh add web1
./onessh ls
./onessh web1
```

When adding a host, you can either:
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

Compatibility note: legacy `hosts.<alias>.auth` is still read as fallback for older configs.

Non-interactive host update examples:

```bash
onessh update ais --alias pi
onessh update ais --host 10.0.0.12 --port 2222
onessh update ais --user-ref ops
onessh update ais --user ubuntu --auth-type key --key-path ~/.ssh/id_ed25519
```

## Security Notes

- Encryption: Argon2id + AES-256-GCM
- Only encrypted data is stored on disk (Git-friendly)
- Master password and plaintext only exist in memory at runtime

## Automated Release (GitHub Actions)

This repository includes a `release` workflow:

- Trigger: push tag `v*` (for example `v0.1.0`)
- Actions:
  - Build multi-platform binaries (Linux/macOS/Windows, amd64/arm64)
  - Create GitHub Release and checksums automatically
  - Update Homebrew formula (`Formula/onessh.rb`) automatically

Release example:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Before first release, ensure repository setting `Actions > Workflow permissions` is set to **Read and write permissions** so formula updates can be pushed.

---

## 中文说明

OneSSH 是一个 Go 实现的 SSH 主机管理 CLI，使用单一主密码对整个配置文件进行加密。

### 功能

- `onessh init` 初始化加密配置
- `onessh add <alias>` 添加主机
- `onessh update <alias>` 更新主机（可交互修改，也可通过通用参数修改）
- `onessh rm <alias>` 删除主机
- `onessh ls` 列出主机别名
- `onessh user ls` 列出可复用 user
- `onessh user add <alias> --name <user>` 新增可复用 user（含认证信息）
- `onessh user update <alias>` 更新可复用 user 的认证/配置
- `onessh user rm <alias>` 删除可复用 user
- `onessh logout` 清除主密码缓存
- `onessh version` 查看版本/构建信息
- `onessh dump` 输出解密后的 YAML 到标准输出
- `onessh <alias>` 或 `onessh connect <alias>` 通过 SSH 连接
- 支持复用用户配置：Host 可通过 `user_ref` 关联独立用户
- 认证信息在 user profile 层维护（而不是 host 层）
- 主密码默认缓存 10 分钟，期间无需重复输入

### 构建

```bash
go build -o onessh ./cmd/onessh
```

### 安装（Homebrew）

发布流水线会自动更新 `Formula/onessh.rb`。

```bash
brew tap tiangong-dev/onessh https://github.com/tiangong-dev/onessh
brew install tiangong-dev/onessh/onessh
```

升级：

```bash
brew update
brew upgrade onessh
```

### 配置文件

默认路径：

```text
~/.onessh/config.enc
```

覆盖方式：

- 环境变量：`ONESSH_CONFIG`
- 命令参数：`--config /path/to/config.enc`
- 命令参数：`--cache-ttl 10m`（默认 10 分钟）
- 命令参数：`--no-cache` 可禁用缓存
- 环境变量：`ONESSH_CACHE_FILE` 可指定缓存文件路径

### 快速开始

```bash
./onessh init
./onessh add web1
./onessh ls
./onessh web1
```

添加主机时可以：
- 输入新的 user（会创建可复用 user profile），或
- 直接选择一个已有的 user profile。

User profile 的 YAML 结构示例：

```yaml
users:
  ops:
    name: ubuntu
    auth:
      type: key
      key_path: ~/.ssh/id_ed25519
```

兼容说明：旧配置中的 `hosts.<alias>.auth` 仍会作为回退逻辑被读取。

非交互更新示例：

```bash
onessh update ais --alias pi
onessh update ais --host 10.0.0.12 --port 2222
onessh update ais --user-ref ops
onessh update ais --user ubuntu --auth-type key --key-path ~/.ssh/id_ed25519
```

### 安全说明

- 加密方案：Argon2id + AES-256-GCM
- 磁盘仅保存密文，适合进入 Git 管理
- 主密码与明文仅在运行时内存中存在

### 自动发布（GitHub Actions）

仓库已配置 `release` workflow：

- 触发条件：推送 tag `v*`（例如 `v0.1.0`）
- 自动执行：
  - 构建多平台二进制（Linux/macOS/Windows, amd64/arm64）
  - 自动创建 GitHub Release 与 checksums
  - 自动更新 Homebrew Formula（`Formula/onessh.rb`）

发布示例：

```bash
git tag v0.1.0
git push origin v0.1.0
```

首次发布前请确认仓库设置 `Actions > Workflow permissions` 为 **Read and write permissions**，否则公式回写会失败。
