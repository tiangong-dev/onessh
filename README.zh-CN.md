# onessh

[English](README.md)

OneSSH 是一个以单一主密码为核心的 CLI SSH 管理工具。所有主机地址、凭证和配置均加密存储——解锁一次，即可连接主机、执行远程命令、传输文件，全程无需重复输入密码。

## 功能

**加密凭证管理**
- `onessh init` 初始化加密配置
- `onessh passwd` 修改主密码
- `onessh logout` 清除主密码缓存
- `onessh agent start|stop|status` 管理内存缓存 agent
- `onessh dump` 输出解密后的 YAML（默认脱敏，`--show-secrets` 显示原始值）

**主机管理**
- `onessh add <alias>` 添加主机
- `onessh update <alias>` 更新主机（交互式或通过参数）
- `onessh rm <alias>` 删除主机
- `onessh ls [--tag <tag>]` 列出主机详情；支持按标签过滤
- `onessh user ls` 列出可复用 user profile
- `onessh user add <alias>` 新增 user profile（含认证信息）
- `onessh user update <alias>` 更新 user profile
- `onessh user rm <alias>` 删除 user profile
- `onessh sshconfig export|import` 与 `~/.ssh/config` 互通

**SSH 操作**
- `onessh <alias> [-- <ssh-args...>]` 或 `onessh connect <alias>` 交互式连接（支持 SSH 参数透传）
- `onessh exec <alias> <command> [args...]` 非交互式执行远程命令，stdout/stderr 直接透传
- `onessh cp <src> <dst>` 通过 scp 传输文件，使用 `alias:path` 格式指定远端路径
- `onessh test [<alias>]` 检测 SSH 连通性；`--all` 可批量检测所有主机

**其他**
- `onessh completion bash|zsh|fish|powershell` 生成 shell 补全脚本（可 Tab 补全主机别名）
- `onessh version` 查看版本/构建信息

**主机级特性**
- 通过 `user_ref` 关联可复用的 user profile，认证信息集中在 profile 层维护
- 支持 Host 级 `env`（注入本地 SSH 进程，并通过 `SendEnv` 转发）
- 支持 `pre_connect` / `post_connect` 钩子，在连接前后执行远端指令
- 支持 `tags` 标签，用于主机分组和过滤
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
onessh connect web1
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
onessh cp -r web1:/var/log/app ./logs      # 递归下载
```

### 连通性检测

```bash
onessh test web1
onessh test --all
onessh test --all --timeout 3
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
```

### Hook 行为说明

- `pre_connect` 先执行，再进入交互式 shell，shell 退出后再执行 `post_connect`。
- 想直接切换到 root shell：`--pre-connect "exec sudo su -"`。

### SSH 配置互通

```bash
onessh sshconfig export
onessh sshconfig export --stdout
onessh sshconfig import
onessh sshconfig import --overwrite
```

- `export` 将 onessh 管理块写入 `~/.ssh/config`。
- `import` 读取可兼容的 `Host` 条目（通配符条目会被忽略）。

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
- 环境变量：`ONESSH_AGENT_SOCKET`

内存 agent 行为：

- 主密码缓存仅支持内存 agent（不兼容文件缓存）。
- 首次成功输入主密码后自动拉起 agent。
- 也可手动管理：`onessh agent start|status|stop`。

密码认证说明：

- 优先使用 `sshpass -d`（基于 FD 传递，不暴露环境变量）。
- 若无 `sshpass`，回退到 `SSH_ASKPASS` + onessh agent IPC token（短时有效）。

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
- 详细机制与流程图见：[`docs/security.md`](docs/security.md)

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
