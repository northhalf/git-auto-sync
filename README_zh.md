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
- **守护模式**：`git-auto-sync daemon add <仓库路径>` 启动后台服务，持续监控仓库。`daemon run`、`daemon stop` 和 `daemon uninstall` 控制服务的启动、停止与卸载。
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

### 合并冲突

Git Auto Sync 使用变基（rebase）而非合并（merge）。如果发生变基冲突，它会中止变基、发送桌面通知，并停止同步该仓库，直到你手动解决冲突。

### 忽略文件

已被 Git 追踪的文件始终参与同步，无视任何忽略规则。未追踪路径中，任一组件以 `.` 开头的路径会被排除在提交和文件系统监控之外；但 `.github/` 中的内容、任意层级的 Git 控制文件（`.gitignore`、`.gitattributes`、`.gitmodules`、`.gitkeep` 和 `.mailmap`），以及文件名以 `.example` 结尾的文件仍可处理。空文件（除了 `.gitkeep`）、被 Git 忽略的文件、Git 元数据和编辑器产生的交换/备份文件（如 Vim、Emacs）仍会被排除，即使它们符合上述例外。

### 嵌套仓库

工作区内发现的嵌套 Git 仓库会被检测并跳过，因此绝不会作为嵌入式 gitlink（mode `160000`）被暂存或提交。这适用于任意嵌套仓库，包括在 `.claude/worktrees/` 下创建的 linked worktree。嵌套仓库内部的变更属于该仓库本身，而不属于正在同步的仓库。

### 日志轮转限制

CLI 和守护进程使用不同的轮转日志文件。多个 CLI 进程（例如正在运行的 `watch` 和手动执行的 `sync`）仍会共同写入 `git-auto-sync.log`。日志轮转库不支持跨进程协调，因此多个 CLI 进程并发运行时，日志文件可能超过配置的轮转大小，也可能在轮转期间丢失部分日志。

## 许可

Apache-2.0
