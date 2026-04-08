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

环境变量回退：

- `ONESSH_AGENT_SOCKET`（回退：`SHUSH_SOCKET`）
- `ONESSH_AGENT_CAPABILITY`（回退：`SHUSH_CAPABILITY`）

主密码仅保存在内存 agent 中；首次成功解锁后可自动拉起 agent；未显式配置时，套接字与 capability 默认由父 shell PID 派生，以隔离不同终端。

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

## 密码认证

- 优先使用 `sshpass -d`（通过文件描述符，不经环境变量暴露）。
- 否则使用 `SSH_ASKPASS` + onessh agent 短时 token。

加密细节与运行时安全见 [安全](/zh/reference/security)。
