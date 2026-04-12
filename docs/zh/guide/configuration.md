# 配置

## 数据目录

默认：

```text
~/.config/onessh/data
```

覆盖方式：

- 环境变量：`ONESSH_DATA`
- 命令行：`--data /path/to/data`

## 缓存与 agent

- `--cache-ttl 10m` — 主密码缓存时长（默认 10 分钟）
- `--no-cache` — 禁用缓存
- `--agent-socket /path/to/agent.sock` — agent Unix 套接字
- `--agent-capability <token>` — 为 agent IPC 启用 capability 校验

用于默认值的的环境变量（未设置对应命令行参数时）：

- `ONESSH_AGENT_SOCKET`
- `ONESSH_AGENT_CAPABILITY`

主密码仅保存在内存 agent 中；首次成功解锁后可自动拉起 agent；未显式配置时，套接字与 capability 默认由父 shell PID 派生，用于便利性和默认命名空间区分，不应视为同 UID 下的强安全边界。

## 存储布局

```text
~/.config/onessh/data/
  meta.yaml
  users/
    <alias>.yaml
  hosts/
    <alias>.yaml
```

敏感字段为 `ENC[...]` 密文，目录结构便于 Git diff。

## 主机条目字段（YAML）

每台主机还可包含：

- **`proxy_jump`** — 传给 `ssh` / `scp` 的 `-J`（跳板）。
- **`env`** — 合并进 SSH 进程的环境变量；键名还会通过 `SendEnv` 发往远端（需远端 `sshd` 允许）。
- **`pre_connect` / `post_connect`** — 在远程交互会话前后执行的本地钩子命令（包在登录 shell 包装里）。与 SSH `-N`、`-T` 不兼容（OneSSH 会拒绝该组合）。

通过 `onessh add` / `onessh update` 编辑；完整模型见 [命令](/zh/guide/commands) 与 [架构](/zh/reference/architecture)。

## 密码认证

- 优先使用 `sshpass -d`（通过文件描述符，不经环境变量暴露）。
- 否则使用 `SSH_ASKPASS` + onessh agent 短时、单次使用 token。该回退路径弱于 `sshpass -d`，因为辅助程序在运行时仍需凭短时 token 向 agent 取回密码。

加密细节与运行时安全见 [安全](/zh/reference/security)。
