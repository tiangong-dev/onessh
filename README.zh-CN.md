# onessh

[English](README.md)

**文档：** [https://tiangong-dev.github.io/onessh/zh/](https://tiangong-dev.github.io/onessh/zh/)（[English](https://tiangong-dev.github.io/onessh/)）

OneSSH 是以单一主密码为核心的 CLI SSH 管理工具。主机、凭证与配置均静态加密（Argon2id + AES-256-GCM）；解锁一次后即可连接、执行命令、传输文件而无需重复输入密钥。数据目录可安全推送到**公开** Git 仓库——磁盘仅存 `ENC[...]` 密文；请使用**高强度主密码**（建议 12 位以上，混合大小写、数字与符号）。

**完整说明：** 命令表、配置、批量操作示例、架构与安全见 **[文档站点](https://tiangong-dev.github.io/onessh/zh/)**。本 README 仅保留构建、安装、发布与极简上手。

## 快速上手

```bash
onessh init
onessh add web1
onessh ls
onessh web1
```

## 构建

```bash
make build
```

带版本信息的本地发布风格构建：

```bash
make build-release VERSION=v0.0.0
```

## 测试

```bash
make test          # 含 e2e
make test-short    # 跳过 e2e
make test-e2e      # 仅 e2e
```

## 安装（Homebrew）

```bash
brew tap tiangong-dev/onessh https://github.com/tiangong-dev/onessh
brew install tiangong-dev/onessh/onessh
```

## 设计文档（源码）

- [架构](docs/reference/architecture.md) — 模块与执行流程  
- [安全](docs/reference/security.md) — 威胁模型与缓解措施  

## 自动发布

推送 tag `v*`（如 `v0.2.0`）可触发多平台构建、GitHub Release 与 Homebrew 公式更新。首次发布前请将 **Actions → Workflow permissions** 设为 **Read and write**。

```bash
git tag v0.2.0
git push origin v0.2.0
```
