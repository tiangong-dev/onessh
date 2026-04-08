# 命令

## 凭证与存储

| 命令 | 说明 |
| --- | --- |
| `onessh init` | 初始化加密配置 |
| `onessh passwd` | 修改主密码 |
| `onessh add <alias>` | 添加主机 |
| `onessh update <alias>` | 更新主机 |
| `onessh rm <alias>` | 删除主机 |
| `onessh ls [--tag] [--filter]` | 列出主机 |
| `onessh show <alias>` | 查看主机详情 |
| `onessh user`（`ls`、`add`、`update`、`rm`） | 管理 user profile |
| `onessh logout [--all]` | 清除主密码缓存 |
| `onessh log [...]` | 审计日志；`enable` / `disable` / `status` |
| `onessh agent`（`start`、`stop`、`status`、`clear-all`） | 内存缓存 agent |

## SSH 操作

| 命令 | 说明 |
| --- | --- |
| `onessh <alias> [-- ssh-args...]` | 交互式 SSH |
| `onessh exec <alias> <cmd> [args...]` | 非交互执行远程命令 |
| `onessh exec --tag <tag> <cmd>` | 按标签批量执行 |
| `onessh cp <src>... <dst>` | 类 scp 传输（`alias:path`） |
| `onessh cp --tag <tag> files... :/path` | 按标签批量上传 |
| `onessh test [<alias>]` | 连通性；支持 `--all`、`--tag`、`--filter` |
| `onessh completion`（`bash`、`zsh`、`fish`、`powershell`） | Shell 补全 |
| `onessh version` | 版本与构建信息 |

## 批量选择器

远程相关命令支持 `--all`、`--tag <tag>`、`--filter <glob>`（Go `filepath.Match`，**整串**匹配）。在支持的命令中，标签与 filter 可组合为 AND。使用 `--dry-run` 可仅列出匹配主机而不执行。

更完整的示例见仓库 [README.zh-CN](https://github.com/tiangong-dev/onessh/blob/main/README.zh-CN.md)。
