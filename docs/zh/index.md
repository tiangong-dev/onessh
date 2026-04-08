---
layout: home

hero:
  name: OneSSH
  text: 加密 SSH 配置，单一主密码
  tagline: 连接、执行命令、传输文件而无需反复输入凭证。数据目录可安全纳入公开 Git 仓库。
  actions:
    - theme: brand
      text: 快速开始
      link: /zh/guide/getting-started
    - theme: alt
      text: 参考文档
      link: /zh/reference/
    - theme: alt
      text: English
      link: /
    - theme: alt
      text: GitHub
      link: https://github.com/tiangong-dev/onessh

features:
  - title: 静态加密存储
    details: Argon2id 与 AES-256-GCM。磁盘仅存 ENC[...] 密文。
  - title: 内存缓存
    details: 本地 agent 按 TTL 缓存主密码；可选 IPC capability 加固。
  - title: 完整 SSH 工作流
    details: 交互式 SSH、exec、类 scp 的 cp、连通性检测，支持按标签或 glob 批量操作。
---

## 文档结构

| 章节 | 内容 |
| --- | --- |
| [指南](/zh/guide/getting-started) | 安装、首台主机、命令表、配置说明 |
| [参考](/zh/reference/) | 架构与安全设计（深度） |

命令与批量操作示例见仓库中的 [README.zh-CN](https://github.com/tiangong-dev/onessh/blob/main/README.zh-CN.md)（长文备用）。

## 快速示例

```bash
onessh init
onessh add web1
onessh ls
onessh web1
```
