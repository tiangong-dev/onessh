# onessh

English | [中文](#中文说明)

OneSSH is a Go-based SSH host manager that encrypts the entire configuration with a single master password.

## Features

- `onessh init` initialize encrypted config
- `onessh add <alias>` add a host
- `onessh update <alias>` update a host
- `onessh rm <alias>` remove a host
- `onessh list` list host aliases
- `onessh user list` list reusable users
- `onessh user add <alias> --name <user>` add a reusable user
- `onessh user rm <alias>` remove a reusable user
- `onessh dump` print decrypted YAML to stdout
- `onessh <alias>` or `onessh connect <alias>` connect via SSH
- Reusable user profiles: hosts can reference shared users (`user_ref`)

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

## Quick Start

```bash
./onessh init
./onessh add web1
./onessh list
./onessh web1
```

When adding a host, you can either:
- create a new user profile by entering a username, or
- select an existing user profile from the list.

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
- `onessh update <alias>` 更新主机
- `onessh rm <alias>` 删除主机
- `onessh list` 列出主机别名
- `onessh user list` 列出可复用 user
- `onessh user add <alias> --name <user>` 新增可复用 user
- `onessh user rm <alias>` 删除可复用 user
- `onessh dump` 输出解密后的 YAML 到标准输出
- `onessh <alias>` 或 `onessh connect <alias>` 通过 SSH 连接
- 支持复用用户配置：Host 可通过 `user_ref` 关联独立用户

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

### 快速开始

```bash
./onessh init
./onessh add web1
./onessh list
./onessh web1
```

添加主机时可以：
- 输入新的 user（会创建可复用 user profile），或
- 直接选择一个已有的 user profile。

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
