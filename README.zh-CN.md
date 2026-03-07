# onessh

[English](README.md)

OneSSH 是一个以单一主密码为核心的 CLI SSH 管理工具。所有主机地址、凭证和配置均加密存储——解锁一次，即可连接主机、执行远程命令、传输文件，全程无需重复输入密码。

数据目录可以安全地推送到**公开的** Git 仓库——所有敏感字段均通过 AES-256-GCM 加密，密钥由 Argon2id 从主密码派生，磁盘上仅保存 `ENC[...]` 密文，主密码永不落盘。请使用**高强度主密码**（建议 12 位以上，混合大小写、数字和符号）以确保抗暴力破解能力。

## 核心 — 加密凭证管理

- `onessh init` 初始化加密配置
- `onessh passwd` 修改主密码
- `onessh add <alias>` 添加主机（交互式或通过参数）
- `onessh update <alias>` 更新主机
- `onessh rm <alias>` 删除主机
- `onessh ls [--tag <tag>] [--filter <glob>]` 列出主机详情；支持按标签或 glob 模式过滤
- `onessh show <alias>` 查看单个主机的详细信息
- `onessh user ls / add / update / rm` 管理可复用 user profile
- `onessh logout [--all]` 清除主密码缓存（或清空所有主密码缓存）
- `onessh agent start|stop|status|clear-all` 管理内存缓存 agent
- 通过 `user_ref` 关联可复用 user profile，认证信息集中在 profile 层维护
- 支持 Host 级 `env`、`pre_connect` / `post_connect` 钩子、`tags` 标签
- 主密码默认缓存 10 分钟，期间无需重复输入

## 附加 — SSH 操作

- `onessh <alias> [-- <ssh-args...>]` 交互式连接（支持 SSH 参数透传）
- `onessh exec <alias> <command> [args...]` 非交互式执行远程命令，stdout/stderr 直接透传
- `onessh exec --tag <tag> <command>` 按标签批量执行命令
- `onessh cp <src>... <dst>` 通过 scp 传输文件，使用 `alias:path` 格式；支持多文件上传和远端到远端复制
- `onessh cp --tag <tag> <files>... :/path` 按标签批量上传文件
- `onessh test [<alias>]` 检测 SSH 连通性；支持 `--all`、`--tag`、`--filter` 批量检测
- `onessh completion bash|zsh|fish|powershell` 生成 shell 补全脚本（Tab 补全主机别名）
- `onessh version` 查看版本/构建信息

## 构建

```bash
make build
```

本地发布风格构建（带版本 ldflags）：

```bash
make build-release VERSION=v0.0.0
```

## 测试

```bash
make test          # 包含 e2e 测试
make test-short    # 快速路径（跳过 e2e）
make test-e2e      # 仅运行 e2e
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

## 快速开始

```bash
onessh init
onessh add web1
onessh ls
onessh web1
onessh web1 -- -L 8080:127.0.0.1:80 -N
```

添加主机时，可以输入新 user（创建新 profile），也可以直接选择已有 profile。

## Shell 补全

```bash
# zsh
onessh completion zsh > "${fpath[1]}/_onessh"

# bash
onessh completion bash > /etc/bash_completion.d/onessh

# fish
onessh completion fish > ~/.config/fish/completions/onessh.fish
```

启用后，`onessh <Tab>` 会通过 agent 缓存补全主机别名，无需输入主密码。

## 主机操作

### 连接

```bash
onessh web1
onessh web1 -- -L 8080:127.0.0.1:80 -N
```

### 执行远程命令

```bash
onessh exec web1 uptime
onessh exec web1 df -h /
onessh exec web1 -- bash -c "cd /srv && ls"
```

### 传输文件

```bash
onessh cp web1:/etc/hosts ./hosts          # 下载
onessh cp ./deploy.sh web1:/tmp/           # 上传
onessh cp file1 file2 web1:/tmp/           # 多文件上传
onessh cp -r web1:/var/log/app ./logs      # 递归下载
onessh cp web1:/etc/hosts web2:/tmp/       # 远端到远端
```

### 连通性检测

```bash
onessh test web1
onessh test --all
onessh test --all --timeout 3
```

### 查看主机详情

```bash
onessh show web1
```

## 批量操作

远程操作命令支持通过 `--all`、`--tag`、`--filter` 进行批量执行。

### `--filter` glob 模式

`--filter` 接受 glob 模式（Go `filepath.Match` 语法），匹配目标为主机别名、主机地址或描述（OR 逻辑——命中任一即通过）。

支持的通配符：

- `*` 匹配任意长度的任意字符
- `?` 匹配单个任意字符
- `[abc]` 匹配集合中的任一字符
- `[a-z]` 匹配范围内的任一字符

注意：这是**整串匹配**而非子串匹配。子串匹配请使用 `*子串*`。

### 示例

```bash
# 批量执行命令
onessh exec --all uptime
onessh exec --tag prod uptime
onessh exec --filter "web*" -- df -h /
onessh exec --tag prod --filter "cn-*" uptime    # tag 与 filter 组合（AND 逻辑）

# 批量连通性检测
onessh test --all
onessh test --tag prod
onessh test --filter "192.168.*"

# 批量上传
onessh cp --tag prod deploy.sh :/tmp/
onessh cp --filter "web*" app.conf :/etc/app/
onessh cp --tag prod -r dist/ :/srv/app/
```

### Dry Run（预览模式）

添加 `--dry-run` 可预览匹配的主机列表，不实际执行操作：

```bash
onessh exec --tag prod --dry-run uptime
onessh cp --filter "web*" --dry-run app.conf :/etc/app/
onessh test --all --dry-run
```

## 主机管理

### 添加主机并打标签

```bash
onessh add web1 --tag prod --tag cn
onessh add staging --tag staging
```

### 更新主机

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

### 列出与过滤

```bash
onessh ls
onessh ls --tag prod
onessh ls --filter "web*"
onessh ls --tag prod --filter "cn-*"
```

### Hook 行为说明

- `pre_connect` 先执行，再进入交互式 shell，shell 退出后再执行 `post_connect`。
- 想直接切换到 root shell：`--pre-connect "exec sudo su -"`。

## 配置

默认数据目录：

```text
~/.config/onessh/data
```

覆盖方式：

- 环境变量：`ONESSH_DATA`
- 命令参数：`--data /path/to/data`
- 命令参数：`--cache-ttl 10m`（默认 10 分钟）
- 命令参数：`--no-cache` 禁用缓存
- 命令参数：`--agent-socket /path/to/agent.sock`
- 命令参数：`--agent-capability <token>` 为 agent IPC 启用 capability 校验
- 环境变量：`ONESSH_AGENT_SOCKET`（回退：`SHUSH_SOCKET`）
- 环境变量：`ONESSH_AGENT_CAPABILITY`（回退：`SHUSH_CAPABILITY`）

内存 agent 行为：

- 主密码缓存仅支持内存 agent（不兼容文件缓存）。
- 首次成功输入主密码后自动拉起 agent。
- 默认会基于父 shell PID 自动派生 agent socket 与 capability（不同窗口通常天然隔离）。
- 也可手动管理：`onessh agent start|status|stop|clear-all`。
- 可使用 `onessh logout --all` 清空 onessh 的全部主密码缓存条目。
- 配置 capability 后，所有 agent 请求（含 askpass token 流程）都必须携带同一 token。

密码认证说明：

- 优先使用 `sshpass -d`（基于 FD 传递，不暴露环境变量）。
- 若无 `sshpass`，回退到 `SSH_ASKPASS` + onessh agent IPC token（默认短时且低次数）。

## 存储结构

```text
~/.config/onessh/data/
  meta.yaml
  users/
    <alias>.yaml
  hosts/
    <alias>.yaml
```

敏感字段以 `ENC[...]` 存储，结构保持可读、便于 Git diff。

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

## 安全说明

- 加密方案：Argon2id + AES-256-GCM
- 磁盘仅保存密文，适合纳入 Git 管理
- 主密码与明文仅在运行时内存中存在
- 架构设计与执行流程见：[`docs/architecture.md`](docs/architecture.md)
- 安全模型与防护细节见：[`docs/security.md`](docs/security.md)

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
