# onessh

OneSSH 是一个 Go 实现的 SSH 配置管理 CLI，使用单一主密码加密整个配置文件。

## 功能

- `onessh init` 初始化加密配置
- `onessh add <alias>` 添加主机
- `onessh update <alias>` 更新主机
- `onessh rm <alias>` 删除主机
- `onessh list` 列出主机别名
- `onessh dump` 导出明文 YAML 到标准输出
- `onessh <alias>` 或 `onessh connect <alias>` 连接 SSH

## 构建

```bash
go build -o onessh ./cmd/onessh
```

## 安装（Homebrew）

发布流水线会自动更新 `Formula/onessh.rb`，可用以下方式安装：

```bash
brew tap tiangong-dev/onessh https://github.com/tiangong-dev/onessh
brew install tiangong-dev/onessh/onessh
```

升级：

```bash
brew update
brew upgrade onessh
```

## 配置文件

默认路径：

```text
~/.onessh/config.enc
```

可通过以下方式覆盖：

- 环境变量：`ONESSH_CONFIG`
- 命令参数：`--config /path/to/config.enc`

## 快速开始

```bash
./onessh init
./onessh add web1
./onessh list
./onessh web1
```

## 安全说明

- 加密方案：Argon2id + AES-256-GCM
- 仅保存密文文件，适合进入 Git 仓库管理
- 主密码与解密明文仅在运行时内存中存在

## 自动发布（GitHub Actions）

仓库已配置 `release` workflow：

- 触发条件：推送 tag（`v*`，例如 `v0.1.0`）
- 行为：
  - 构建多平台二进制（Linux/macOS/Windows, amd64/arm64）
  - 自动创建 GitHub Release 与 checksums
  - 自动更新 Homebrew Formula（`Formula/onessh.rb`）

发布命令示例：

```bash
git tag v0.1.0
git push origin v0.1.0
```

首次启用前请确认仓库设置中 `Actions > Workflow permissions` 为 **Read and write permissions**，否则公式回写会失败。
