<div align="center">
  <div style="width:200px">
    <a href="https://github.com/northhalf/git-auto-sync">
      <img src="assets/icon.webp" alt="Git Auto Sync" width="200">
    </a>
  </div>

<h1>Git Auto Sync</h1>

![Status](https://img.shields.io/badge/status-active-brightgreen) ![Stage](https://img.shields.io/badge/stage-beta-blue) ![Build Status](https://github.com/northhalf/git-auto-sync/actions/workflows/ci.yml/badge.svg) ![Release](https://img.shields.io/github/v/release/northhalf/git-auto-sync) ![Downloads](https://img.shields.io/github/downloads/northhalf/git-auto-sync/total) ![License](https://img.shields.io/badge/license-Apache--2.0-blue)

<p align="center"><a href="./README.md">English</a> | 中文</p>

<h5>一个自动提交并同步 Git 仓库的轻量级命令行工具。</h5>

Git Auto Sync 是 <a href="https://github.com/GitJournal/git-auto-sync"><b>GitJournal/git-auto-sync</b></a> 的修改版本，主要修复原项目的 bug 并完善功能。

</div>

## 使用场景

Git Auto Sync 面向个人仓库：与上游保持一致比提交历史是否整洁更重要。它会用自动生成的信息提交本地改动、变基到上游分支并推送，让编辑器或笔记工作流无需手动提交即可在多台机器间保持镜像同步。它**不**适合依赖有意义、人工撰写提交信息的协作仓库。

## 快速开始

### 环境要求

- Go 1.25 或更高版本
- Git
- 已配置的 Git 身份（`user.name` 和 `user.email`）

### 编译

```bash
git clone https://github.com/northhalf/git-auto-sync.git
cd git-auto-sync
make
```

编译完成后，`git-auto-sync` 和 `git-auto-sync-daemon` 会位于 `./bin` 目录。

### 手动同步

在任意 Git 仓库中执行：

```bash
/path/to/bin/git-auto-sync sync
```

这会提交符合条件的改动、拉取所有远程、变基到配置的上游分支并推送。

### 后台守护进程

注册需要持续监控的仓库：

```bash
/path/to/bin/git-auto-sync daemon add /path/to/repo
```

查看状态：

```bash
/path/to/bin/git-auto-sync daemon status
```

守护进程会监听文件系统、按配置的间隔轮询，并自动同步。

## 使用说明

Git Auto Sync 提供两种工作模式：

- **手动模式**：`git-auto-sync sync` 立即执行一次完整同步流程。
- **守护模式**：`git-auto-sync daemon add <仓库路径>` 启动后台服务，持续监控仓库。`daemon run`、`daemon stop`、`daemon restart` 和 `daemon uninstall` 控制服务的启动、停止、重启与卸载。
- **设置**：`git-auto-sync config <键> [值]` 读取、设置或删除 `syncInterval`、`debounce`、`gitexec`，支持 `--global`（默认）或 `--local` 作用域。

运行 `git-auto-sync --help` 或 `git-auto-sync daemon --help` 查看所有命令。

### 仓库配置

设置分为两级：全局（位于 `~/.config/git-auto-sync/config.json`）与仓库级（位于 Git 配置的 `[auto-sync]` 段）。仓库级覆盖全局，全局覆盖默认值。时间单位为分钟。

```bash
git-auto-sync config syncInterval 60          # 分钟，默认 60（全局）
git-auto-sync config --local syncInterval 30  # 仓库级覆盖
git-auto-sync config --local debounce 5       # 分钟，默认 10
git-auto-sync config --global gitexec /usr/bin/git  # 默认：通过 PATH 查找 git
git-auto-sync config --list                   # 查看生效设置
git-auto-sync config --unset syncInterval     # 删除设置（默认：全局）
```

默认值：每小时同步一次、防抖 10 分钟、`git` 通过 PATH 查找。

### 防抖

守护进程使用 `debounce` 设置（默认 10 分钟）对文件系统和唤醒事件进行防抖。每个事件都会重置计时器，只有在配置的静默期内没有新事件时才执行一次同步，因此一连串编辑会合并为一次提交，而不是每次保存都提交。来自 `syncInterval` 的周期性触发和机器唤醒事件绕过防抖、立即触发同步，因此计划同步和唤醒同步不会被延迟。在同步进行期间到达的触发会合并为同步结束后的一次后续同步。

### 提交信息

每条提交信息都由 `git status --porcelain` 生成。每个符合条件的改动会变成一行 `XY 路径`，其中 `XY` 是两位 Git 状态码（例如 `M`、`A` 或 `??`），`路径` 是相对于仓库的路径。这些行按字母排序、以换行连接，再传给 `git commit -m`：

```
?? notes/2026-07-18.md
M src/main.go
A docs/changelog.md
```

其中没有任何人工撰写的摘要。这正是该工具适合“重同步、轻历史”工作流的原因（见[使用场景](#使用场景)）。

### 何时暂停同步

Git Auto Sync 使用变基（rebase）而非合并（merge）。某些情况会使守护进程停止同步某个仓库，直到你介入处理。检测到任一情况时，它会发送桌面通知并暂停该仓库；恢复方式是解决问题后**重启守护进程**（或移除并重新添加该仓库）：

- **有 Git 操作正在进行** —— 未完成的 merge、rebase、cherry-pick 或 revert。
- **分离头指针（detached HEAD）** —— HEAD 不在任何分支上，因此没有可变基或推送的目标。
- **没有上游** —— 当前分支未配置上游追踪分支。
- **缺少 Git 身份** —— 未设置 `user.name` 或 `user.email`。
- **变基冲突** —— 变基到上游时发生冲突；变基会被中止，仓库在推送前暂停。

来自 `fetch` 和 `push` 的网络错误**不会**暂停仓库。守护进程会以有上限的退避策略重试（2、4、8、15、30，然后 60 分钟），并在远程恢复可达后自动继续。

### 忽略文件

已被 Git 追踪的文件始终参与同步，无视任何忽略规则。未追踪路径中，任一组件以 `.` 开头的路径会被排除在提交和文件系统监控之外；但 `.github/` 中的内容、任意层级的 Git 控制文件（`.gitignore`、`.gitattributes`、`.gitmodules`、`.gitkeep` 和 `.mailmap`），以及文件名以 `.example` 结尾的文件仍可处理。空文件（除了 `.gitkeep`）、被 Git 忽略的文件、Git 元数据和编辑器产生的交换/备份文件（如 Vim、Emacs）仍会被排除，即使它们符合上述例外。

如果你想同步某个默认被排除的路径（例如不是 Git 控制文件的点文件），请自行用 `git add` 暂存它。一旦 Git 追踪了该文件，它就始终符合条件，并绕过上文所有忽略规则。

### 嵌套仓库

工作区内发现的嵌套 Git 仓库会被检测并跳过，因此绝不会作为嵌入式 gitlink（mode `160000`）被暂存或提交。这适用于任意嵌套仓库，包括在 `.claude/worktrees/` 下创建的 linked worktree。嵌套仓库内部的变更属于该仓库本身，而不属于正在同步的仓库。

### Git 与 Git LFS

Git Auto Sync 仅用 go-git 做只读仓库检查（仓库发现、HEAD 与分支配置、忽略匹配、作者校验）。所有变更和网络操作 —— `status`、`add`、`commit`、`fetch`、`rebase`、`push` —— 都通过 `git` 可执行文件执行（经 `PATH` 或 `gitexec` 设置解析）。因此必须安装可用的 `git`。

如果你的仓库使用 Git LFS，请自行安装 `git-lfs` 扩展，以便 Git 的 clean/smudge 过滤器正常运行。Git Auto Sync 能识别 `git status` 报告 LFS 指针已修改但 `git add` 未暂存任何内容的情况（例如 `GIT_LFS_SKIP_SMUDGE` 下仅含指针的工作区），并干净地跳过而非让提交失败，但它本身不管理 LFS 对象。

### 日志轮转限制

CLI 和守护进程使用不同的轮转日志文件。多个 CLI 进程（例如正在运行的 `watch` 和手动执行的 `sync`）仍会共同写入 `git-auto-sync.log`。日志轮转库不支持跨进程协调，因此多个 CLI 进程并发运行时，日志文件可能超过配置的轮转大小，也可能在轮转期间丢失部分日志。

## 相比原项目的改进

Git Auto Sync 基于 [GitJournal/git-auto-sync](https://github.com/GitJournal/git-auto-sync)。原项目截至（含）`50cb029` 的提交为基线，此后所有提交均为本项目的工作。主要改进：

- **引擎现代化** —— 从 `src-d/go-git.v4` 迁移到 `go-git/go-git/v5`，现代化 Go 工具链与依赖，并将共享代码重组为职责清晰的 `internal/` 包。
- **变更操作改用 Git CLI** —— `status`、`add`、`commit`、`fetch`、`rebase`、`push` 改为通过 `git` 可执行文件执行，而非 go-git，使内容过滤器（含 Git LFS）行为正确。嵌套仓库会被检测并跳过，而非作为 gitlink 暂存。
- **更智能的同步** —— 解析 HEAD 与上游的状态，跳过多余的变基和推送（相等、本地领先、上游领先或分叉）。
- **仓库状态守护** —— 当仓库有操作进行中、分离头指针或无上游时，在任何变更前暂停，而非在同步中途失败。
- **守护进程加固** —— 对文件改动防抖但不延迟计划同步；按仓库隔离失败，对远程错误使用有上限的退避重试；转发 Linux 与 Windows 唤醒事件，使恢复运行的机器立即同步。
- **统一配置** —— 通过 `config` CLI 与配置文件轮询，在不重启守护进程的情况下重新加载全局与仓库级设置（`syncInterval`、`debounce`、`gitexec`）。
- **改进的忽略规则** —— 已追踪文件始终符合条件；未追踪的隐藏路径被忽略，但有显式例外（`.github/`、Git 控制文件、`*.example`）；忽略匹配会归一化路径，并按同步轮次缓存索引。
- **守护进程与 CLI 体验** —— 新增 `daemon run`、`stop`、`restart`、`uninstall` 命令；`ls` 与 `status` 显示按仓库的运行状态和最近同步时间；为 CLI 与守护进程提供结构化轮转日志；继承完整父环境并对密钥脱敏；修复 Windows 服务，使 LocalSystem 共享用户路径与 Git 配置。

## 许可

Apache-2.0
