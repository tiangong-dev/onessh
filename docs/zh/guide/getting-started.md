# 快速开始

OneSSH 在单一主密码下加密保存 SSH 主机与凭证。首次解锁后（缓存在内存中），日常操作无需反复输入密码。

## 安装

### Homebrew（macOS / Linux）

```bash
brew tap tiangong-dev/onessh https://github.com/tiangong-dev/onessh
brew install tiangong-dev/onessh/onessh
```

### 源码构建

```bash
git clone https://github.com/tiangong-dev/onessh.git
cd onessh
make build
```

可执行文件 `onessh` 会生成在仓库根目录。

## 初始化

```bash
onessh init
```

请使用**高强度主密码**（建议 12 位以上，混合大小写、数字与符号）。主密码不会写入磁盘，仅派生密钥与密文会保存。

## 添加主机

交互式添加：

```bash
onessh add web1
```

可新建 user profile，或通过 `user_ref` 关联已有 profile。

## 连接

```bash
onessh web1
```

在 `--` 之后传入额外 SSH 参数：

```bash
onessh web1 -- -L 8080:127.0.0.1:80 -N
```

## 下一步

- [命令](/zh/guide/commands) — 命令一览
- [配置](/zh/guide/configuration) — 数据目录、agent、环境变量
- [参考](/zh/reference/) — 架构与安全深度说明
