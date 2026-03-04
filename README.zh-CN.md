# onessh

[English](README.md)

OneSSH 是一个 Go 实现的 SSH 主机管理 CLI，使用单一主密码对配置进行加密管理。

## 功能

- `onessh init` 初始化加密配置
- `onessh add <alias>` 添加主机
- `onessh update <alias>` 更新主机（可交互修改，也可通过通用参数修改）
- `onessh rm <alias>` 删除主机
- `onessh ls` 列出主机详情（含 user/auth/port 摘要）
- `onessh user ls` 列出可复用 user
- `onessh user add <alias> --name <user>` 新增可复用 user（含认证信息）
- `onessh user update <alias>` 更新可复用 user 的认证/配置
- `onessh user rm <alias>` 删除可复用 user
- `onessh logout` 清除主密码缓存
- `onessh agent start|stop|status` 管理内存缓存 agent
- `onessh version` 查看版本/构建信息
- `onessh dump` 输出解密后的 YAML 到标准输出
- `onessh sshconfig export|import` 与 `~/.ssh/config` 互通
- `onessh <alias> [-- <ssh-args...>]` 或 `onessh connect <alias> [-- <ssh-args...>]` 通过 SSH 连接（支持 SSH 参数透传）
- Host 通过 `user_ref` 关联独立 user profile
- 认证信息在 user profile 层维护
- 支持 Host 级 `env`（注入本地 SSH 进程，并通过 `SendEnv` 转发）
- 支持 `pre_connect` / `post_connect`（按 Host 预设远端连接前后指令）
- 主密码默认缓存 10 分钟，期间无需重复输入

## 构建

```bash
go build -o onessh ./cmd/onessh
```

## 安装（Homebrew）

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

## 配置文件

默认路径：

```text
~/.onessh/config
```

覆盖方式：

- 环境变量：`ONESSH_CONFIG`
- 命令参数：`--config /path/to/config`
- 命令参数：`--cache-ttl 10m`（默认 10 分钟）
- 命令参数：`--no-cache` 可禁用缓存
- 命令参数：`--agent-socket /path/to/agent.sock`（memory 后端使用）
- 环境变量：`ONESSH_AGENT_SOCKET` 可指定内存 agent 的 socket 路径

memory 后端行为：

- 主密码缓存仅支持内存 agent（不再兼容文件缓存）。
- 第一次成功输入主密码后会自动拉起 agent。
- 也可以手动管理：`onessh agent start|status|stop`。

存储结构（分片 + SOPS 风格值加密）：

```text
~/.onessh/config/
  meta.yaml
  users/
    <alias>.yaml
  hosts/
    <alias>.yaml
```

敏感字段值以 `ENC[...]` 保存，结构保持可读、便于 Git diff。

示例文件：

```yaml
# ~/.onessh/config/users/ops.yaml
version: 1
name: ENC[v1,...]
auth:
  type: key
  key_path: ENC[v1,...]
```

```yaml
# ~/.onessh/config/hosts/ais.yaml
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

## 快速开始

```bash
./onessh init
./onessh add web1
./onessh ls
./onessh web1
./onessh web1 -- -L 8080:127.0.0.1:80 -N
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

Host 条目必须包含 `user_ref`，不在 host 层保存 `auth` / `user` 字段。

非交互更新示例：

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

Hook 行为说明：

- `pre_connect` 先执行，再进入交互式 shell，shell 退出后再执行 `post_connect`。
- 如果你想直接切换到 root shell，可使用 `--pre-connect "exec sudo su -"`.

SSH 配置互通：

```bash
onessh sshconfig export
onessh sshconfig export --stdout
onessh sshconfig import
onessh sshconfig import --overwrite
```

- `export` 会把 onessh 管理块写入 `~/.ssh/config`。
- `import` 会读取可兼容的 `Host` 条目（会忽略通配符条目）。

## 安全说明

- 加密方案：Argon2id + AES-256-GCM
- 磁盘仅保存密文，适合进入 Git 管理
- 主密码与明文仅在运行时内存中存在

## 自动发布（GitHub Actions）

仓库已配置 `release` workflow：

- 触发条件：推送 tag `v*`（例如 `v0.2.0`）
- 自动执行：
  - 构建多平台二进制（Linux/macOS/Windows, amd64/arm64）
  - 自动创建 GitHub Release 与 checksums
  - 自动更新 Homebrew Formula（`Formula/onessh.rb`）

发布示例：

```bash
git tag v0.2.0
git push origin v0.2.0
```

首次发布前请确认仓库设置 `Actions > Workflow permissions` 为 **Read and write permissions**，否则公式回写会失败。
