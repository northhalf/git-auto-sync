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

可在 Linux、macOS、Windows 和 Android（Termux）上运行。

</div>

## 使用场景

Git Auto Sync 面向个人仓库：与上游保持一致比提交历史是否整洁更重要。它会用自动生成的信息提交本地改动、变基到上游分支并推送，让编辑器或笔记工作流无需手动提交即可在多台机器间保持镜像同步。它**不**适合依赖有意义、人工撰写提交信息的协作仓库。

## 快速开始

### 环境要求

- Git
- 已配置的 Git 身份（`user.name` 和 `user.email`）
- 从源码构建还需要 Go 1.25 或更高版本

### 获取二进制

从以下两种方式中任选一种。

<details>
<summary><b>方式 A - 从 release 下载</b></summary>

从 [releases 页面](https://github.com/northhalf/git-auto-sync/releases/latest) 下载对应平台的归档并解压。每个归档包含两个二进制（`git-auto-sync`、`git-auto-sync-daemon`；Windows 下为 `.exe`）以及一个 `completions/` 目录，内含各 shell 的补全脚本。

```bash
# 示例：把 Linux x86_64 的 release 解压到 ~/.local/share/git-auto-sync
mkdir -p ~/.local/share/git-auto-sync
tar -xzf git-auto-sync_*_Linux_x86_64.tar.gz -C ~/.local/share/git-auto-sync
```

在 Linux 和 macOS 上，运行前为解压出的两个二进制添加可执行权限：

```bash
chmod +x /path/to/binaries/git-auto-sync /path/to/binaries/git-auto-sync-daemon
```

如果没有可执行权限，即使目录已加入 `PATH`，shell 也无法运行这些文件。

</details>

<details>
<summary><b>方式 B - 从源码构建</b></summary>

```bash
git clone https://github.com/northhalf/git-auto-sync.git
cd git-auto-sync
make
```

`git-auto-sync` 和 `git-auto-sync-daemon` 会构建到 `./bin` 目录。补全脚本位于 `completions/`。

</details>

### 把程序目录加入 PATH

无论用哪种方式，都要把存放二进制的目录加入 `PATH`，这样可以直接运行 `git-auto-sync`：

```bash
# Linux / macOS - 加入 ~/.bashrc 或 ~/.zshrc
export PATH="$PATH:/path/to/binaries"

# Windows (PowerShell) - 为当前用户设置
[Environment]::SetEnvironmentVariable("PATH", $env:PATH + ";C:\path\to\binaries", "User")
```

### Shell 补全（可选）

`completions/` 目录提供了 bash、zsh 和 PowerShell 的补全脚本。按你的 shell 加载对应脚本（把 `/path/to/completions/` 替换为实际路径 - release 解压后的 `completions/`，或克隆仓库里的 `completions/`）：

```bash
# bash - 加入 ~/.bashrc。需要安装 bash-completion 包。
source /path/to/completions/bash_autocomplete

# zsh - 加入 ~/.zshrc，或把文件放到 $fpath 中的某个目录
source /path/to/completions/zsh_autocomplete

# PowerShell - 在你的 profile 中 dot-source
. C:\path\to\completions\powershell_autocomplete.ps1
```

### 手动同步

在任意 Git 仓库中执行：

```bash
git-auto-sync sync
```

这会提交符合条件的改动、拉取配置的上游分支、变基到该分支并推送。

### 后台守护进程

注册需要持续监控的仓库：

```bash
git-auto-sync daemon add /path/to/repo
```

查看状态：

```bash
git-auto-sync daemon status
```

守护进程会监听文件系统、按配置的间隔轮询，并自动同步。

### Android / Termux

请下载 `Android_arm64` release 归档，不要在 Termux 中使用 `Linux_arm64`。Android 的系统调用策略与普通 Linux 不同，Linux Go 二进制在查找可执行文件时可能因 `SIGSYS` 直接退出。

按需安装运行依赖：

```bash
pkg install git

# 仅 daemon 命令需要。
pkg install termux-services

# 可选：启用 Android 系统通知。
pkg install termux-api
```

安装 `termux-services` 后，重启 Termux，或执行：

```bash
source "$PREFIX/etc/profile.d/start-services.sh"
```

`daemon run` 和 `daemon add` 通过 runit 创建并管理 `$PREFIX/var/service/git-auto-sync-daemon`。`daemon uninstall` 只删除由本程序管理的服务定义，保留应用配置、仓库注册列表、运行状态和日志。程序不会覆盖或删除并非由 Git Auto Sync 创建的同名服务目录。

设置了 `XDG_CONFIG_HOME` 时，全局设置文件位于 `$XDG_CONFIG_HOME/git-auto-sync/config.json`。否则位于 `$HOME/.config/git-auto-sync/config.json`；Termux 的常见完整路径是 `/data/data/com.termux/files/home/.config/git-auto-sync/config.json`。`git-auto-sync config --global ...` 命令会读写该文件。

Android 通知还要求安装与 Termux 来源一致的 Termux:API Android 应用。如果找不到 `termux-notification`，`sync`、`watch` 和 daemon 命令会显示警告，但继续正常运行。Android 没有 systemd-logind 唤醒源，因此唤醒通知明确禁用；文件系统事件和配置的 `syncInterval` 仍会触发同步。

Android 12 及更高版本可能终止 Termux 后台进程。将 Termux 排除在电池优化之外可提高可靠性。`termux-wake-lock` 为可选措施，并会增加耗电。

## 使用说明

Git Auto Sync 提供两种工作模式：

- **手动模式**：`git-auto-sync sync` 立即执行一次完整同步流程。
- **守护模式**：`git-auto-sync daemon add <仓库路径>` 启动后台服务，持续监控仓库。`daemon run`、`daemon stop`、`daemon restart` 和 `daemon uninstall` 控制服务的启动、停止、重启与卸载。

`syncInterval`、`debounce` 和 `gitexec` 通过独立的 `git-auto-sync config <键> [值]` 命令管理，支持 `--global`（默认）或 `--local` 作用域。

运行 `git-auto-sync --help` 或 `git-auto-sync daemon --help` 查看所有命令。

### 仓库配置

设置分为两级：全局（位于平台配置文件，例如 Linux 下的 `~/.config/git-auto-sync/config.json`，见[文件位置](#文件位置)）与仓库级（位于 Git 配置的 `[auto-sync]` 段）。仓库级覆盖全局，全局覆盖默认值。

`config` 命令支持三个设置项：

| 键 | 含义 | 可接受的值 | 默认值 | 作用域 |
| --- | --- | --- | --- | --- |
| `syncInterval` | 周期同步触发之间的间隔。周期触发会立即开始同步，不等待 `debounce`。 | 正整数，单位为分钟 | `60` | `--global` 或 `--local` |
| `debounce` | 最近一次符合条件的文件系统创建、写入、重命名或删除事件结束后，到事件触发同步开始前的静默期。每个新的符合条件事件都会重置计时器；周期触发和机器唤醒触发不受该设置影响。 | 正整数，单位为分钟 | `10` | `--global` 或 `--local` |
| `gitexec` | Git 子进程使用的 Git 可执行文件。 | 已存在的 Git 可执行文件路径 | 通过 `PATH` 解析的 `git` | `--global` 或 `--local` |

`--global` 是默认作用域。`--local` 将值写入当前仓库；`--unset` 删除所选作用域中的值，使设置回退到下一级。仓库注册和 daemon 环境变量分别通过 `daemon add`、`daemon rm` 和 `daemon env` 管理，不属于 `config` 设置项。

```bash
git-auto-sync config syncInterval 60          # 分钟，默认 60（全局）
git-auto-sync config --local syncInterval 30  # 仓库级覆盖
git-auto-sync config --local debounce 5       # 分钟，默认 10
git-auto-sync config --global gitexec /usr/bin/git  # 默认：通过 PATH 查找 git
git-auto-sync config --list                   # 查看生效设置
git-auto-sync config --unset syncInterval     # 删除设置（默认：全局）
```

### 防抖

守护进程递归监听仓库目录树下文件和目录的创建、写入、重命名与删除事件。写入事件包括文件内容修改。守护进程先应用忽略文件规则，因此 Git 元数据、被 Git 忽略的文件、空文件及其他已排除路径产生的事件不会重置防抖计时器。

每个符合条件的文件系统事件都会重置 `debounce` 计时器（默认 10 分钟）。只有在配置的静默期内没有新的符合条件事件时才执行同步，因此一连串编辑会合并为一次提交，而不是每次保存都提交。来自 `syncInterval` 的周期性触发和机器唤醒事件绕过防抖、立即触发同步，因此计划同步和唤醒同步不会被延迟。在同步进行期间到达的触发会合并为同步结束后的一次后续同步。

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

除点前缀约定外，在具备该机制的平台上还会识别操作系统级的隐藏属性，且无文件名例外：

- **Windows** -- 文件本身或任一祖先目录带有 `FILE_ATTRIBUTE_HIDDEN` 属性（通过文件资源管理器属性或 `attrib +H` 设置）时，予以排除。
- **macOS** -- 文件本身或任一祖先目录带有 `UF_HIDDEN` 文件标志（通过 `chflags hidden` 设置）时，予以排除。

祖先目录上的隐藏属性会排除其下所有未追踪路径。已追踪文件仍会绕过这些检查。Linux 没有对等的文件系统属性，因此只适用点前缀约定；GTK 的 `.hidden` 文件约定未予实现，因为它仅限特定桌面环境（Nautilus/Nemo），且基于文件名而非属性。

### 符号链接路径

你可以通过目录符号链接注册仓库，包括 Termux 的 `~/storage` 路径。Git Auto Sync 在设置和状态输出中保留配置路径，但在检查文件系统事件是否位于仓库内时解析仓库根目录。文件监听器无论返回配置中的链接路径还是真实目标路径，程序都会将事件归入同一仓库。删除或重命名事件指向的路径已经不存在时，检查器会解析最近仍存在的父目录，再恢复缺失的路径后缀并判断仓库边界。

路径归一化只用于仓库边界判断。Git Auto Sync 不会重写 `config.json`，不会跟随工作区内部符号链接去同步目标内容，也不会改变 Git 原有的符号链接行为。程序暂不保证将不同的 bind mount 路径、Windows `SUBST` 盘符或网络映射路径识别为同一仓库。

### 嵌套仓库

工作区内发现的嵌套 Git 仓库会被检测并跳过，因此绝不会作为嵌入式 gitlink（mode `160000`）被暂存或提交。这适用于任意嵌套仓库，包括在 `.claude/worktrees/` 下创建的 linked worktree。嵌套仓库内部的变更属于该仓库本身，而不属于正在同步的仓库。

### Git 与 Git LFS

Git Auto Sync 仅用 go-git 做只读仓库检查（仓库发现、HEAD 与分支配置、忽略匹配、作者校验）。所有变更和网络操作 —— `status`、`add`、`commit`、`fetch`、`rebase`、`push` —— 都通过 `git` 可执行文件执行（经 `PATH` 或 `gitexec` 设置解析）。因此必须安装可用的 `git`。

如果你的仓库使用 Git LFS，请自行安装 `git-lfs` 扩展，以便 Git 的 clean/smudge 过滤器正常运行。Git Auto Sync 能识别 `git status` 报告 LFS 指针已修改但 `git add` 未暂存任何内容的情况（例如 `GIT_LFS_SKIP_SMUDGE` 下仅含指针的工作区），并干净地跳过而非让提交失败，但它本身不管理 LFS 对象。

### 日志轮转限制

CLI 和守护进程使用不同的轮转日志文件。多个 CLI 进程（例如正在运行的 `watch` 和手动执行的 `sync`）仍会共同写入 `git-auto-sync.log`。日志轮转库不支持跨进程协调，因此多个 CLI 进程并发运行时，日志文件可能超过配置的轮转大小，也可能在轮转期间丢失部分日志。

### 文件位置

Git Auto Sync 将配置和日志存放在平台相关的目录中。

**配置** -- 全局设置文件保存 `repos`、`envs`、`syncInterval`、`debounce` 和 `gitexec`：

| 平台 | 路径 |
| --- | --- |
| Linux | `~/.config/git-auto-sync/config.json` |
| Android / Termux | 设置了 `XDG_CONFIG_HOME` 时为 `$XDG_CONFIG_HOME/git-auto-sync/config.json`；否则为 `~/.config/git-auto-sync/config.json` |
| macOS | `~/Library/Application Support/git-auto-sync/config.json` |
| Windows | `%AppData%\git-auto-sync\config.json` |

仓库级设置保存在仓库自身的 Git 配置的 `[auto-sync]` 段中，不在此文件内。

**日志** -- CLI 和守护进程各自写入一个轮转日志文件（每个文件 10 MB，保留 3 个备份）：

| 平台 | 目录 |
| --- | --- |
| Linux | `~/.local/share/git-auto-sync/log/` |
| Android / Termux | `~/.local/share/git-auto-sync/log/` |
| macOS | `~/Library/Logs/` |
| Windows | `%LOCALAPPDATA%\git-auto-sync\logs\` |

| 文件 | 写入者 |
| --- | --- |
| `git-auto-sync.log` | `git-auto-sync` CLI |
| `git-auto-sync-daemon.log` | 守护进程服务 |

关于多个 CLI 进程共享同一日志文件的注意事项，见[日志轮转限制](#日志轮转限制)。

### Windows 权限

守护进程以 Windows 服务运行，安装在 `LocalSystem` 账户下。管理该服务的子命令 —— `daemon run`、`stop`、`restart` 和 `uninstall` —— 需要访问 Windows 服务控制管理器，因此必须在**管理员**终端中运行；在非管理员终端中执行会提示 `Access is denied`。

使用这些 `daemon` 子命令前，请以管理员身份运行终端。为方便起见，Windows Terminal 可配置为默认以管理员模式打开某个配置文件：打开配置文件下拉菜单，选择**设置**，选中目标配置文件，然后开启**以管理员身份运行此配置文件**：

![Windows Terminal：将配置文件设置为默认以管理员模式运行](assets/windows-terminal-admin.webp)

## 相比原项目的改进

Git Auto Sync 基于 [GitJournal/git-auto-sync](https://github.com/GitJournal/git-auto-sync)。原项目截至（含）`50cb029` 的提交为基线，此后所有提交均为本项目的工作。主要改进：

- **引擎现代化** —— 从 `src-d/go-git.v4` 迁移到 `go-git/go-git/v5`，现代化 Go 工具链与依赖，并将共享代码重组为职责清晰的 `internal/` 包。
- **变更操作改用 Git CLI** —— `status`、`add`、`commit`、`fetch`、`rebase`、`push` 改为通过 `git` 可执行文件执行，而非 go-git，使内容过滤器（含 Git LFS）行为正确。嵌套仓库会被检测并跳过，而非作为 gitlink 暂存。
- **更智能的同步** —— 解析 HEAD 与上游的状态，跳过多余的变基和推送（相等、本地领先、上游领先或分叉）。
- **仓库状态守护** —— 当仓库有操作进行中、分离头指针或无上游时，在任何变更前暂停，而非在同步中途失败。
- **守护进程加固** —— 对文件改动防抖但不延迟计划同步；按仓库隔离失败，对远程错误使用有上限的退避重试；转发 Linux 与 Windows 唤醒事件，使恢复运行的机器立即同步。
- **统一配置** —— 通过 `config` CLI 与配置文件轮询，在不重启守护进程的情况下重新加载全局与仓库级设置（`syncInterval`、`debounce`、`gitexec`）。
- **改进的忽略规则** —— 已追踪文件始终符合条件；未追踪的隐藏路径被忽略，但有显式例外（`.github/`、Git 控制文件、`*.example`）；忽略匹配会归一化路径，并按同步轮次缓存索引。
- **守护进程与 CLI 体验** —— 新增 `daemon run`、`stop`、`restart`、`uninstall` 命令；为 CLI 与守护进程提供结构化轮转日志；继承完整父环境并对密钥脱敏；修复 Windows 服务，使 LocalSystem 共享用户路径与 Git 配置。
- **监控列表可视化** —— 与原项目不同，`daemon ls` 和 `daemon status` 会把每个受监控的仓库渲染为一张对齐的表格：实时显示其运行状态（`running`、`paused (<原因>)` 或 `unknown (daemon may not be running)`）和最近同步时间（`never synced`，或诸如 `synced 3m ago` 的相对时长），表头同时报告守护进程服务状态。哪些仓库健康、已暂停或已过期，一目了然。
- **Android / Termux 支持** —— 专用 Android ARM64 release 避免 Linux 系统调用不兼容；daemon 通过 `termux-services` 交给 runit 管理；Android 通知使用 `termux-notification`；可选通知依赖缺失时降级为警告。

## 原项目缺陷修复

除上文的改进外，原 `GitJournal/git-auto-sync` 基线（commit `50cb029`）中存在的以下缺陷已修复：

### Windows 服务

- **Windows 下守护进程服务无法启动** -- 服务的可执行文件路径注册时未带 `.exe` 后缀，而 Windows 启动服务时不会自动追加该后缀，导致服务无法启动。路径现已按目标平台加上正确后缀。(`2f781ef`)
- **Windows 守护进程以 LocalSystem 身份运行，丢失用户路径与 Git 配置** -- 安装的服务以 `LocalSystem` 运行，其空白的系统配置文件导致守护进程无法写日志、解析仓库、在 `PATH` 中找到 `git`、读取用户 Git 身份，或操作用户的工作区（`dubious-ownership`）。安装逻辑现会注入用户的 `APPDATA`、`LOCALAPPDATA`、`USERPROFILE` 和 `Path`，并在 Windows 下为每个仓库传递 `-c safe.directory=<repo>`。(`6c21dbf`)

### 提交与 Git LFS

- **未变更的 Git LFS 文件每次同步都被重复提交，且 `check` 在路径为空或根路径时崩溃** -- go-git 的状态读取与暂存不经过 Git 的 clean/smudge 过滤器，导致指针未变更的 LFS 跟踪文件被判定为已修改、每个周期都被重新提交；`ShouldIgnoreFile` 在空路径或仓库根路径上还会越界切片导致 panic。现已跳过 LFS 指针，并对空路径和根路径加以校验。(`906d831`)
- **嵌套 Git 仓库被作为嵌入式 gitlink 暂存，链接工作区被递归进入** -- 原项目对任何未忽略的未跟踪路径都通过 go-git 暂存，包括嵌套仓库（被作为 mode `160000` 的 gitlink 提交）和 `git worktree` 目录。提交阶段现改用 `git status --porcelain` 与 `git add`，检测并跳过嵌套仓库。(`d3c6e0f`)

### 忽略规则

- **针对嵌套或绝对路径的 Git 忽略规则失效** -- 完整路径被作为单个组件传给 go-git 的忽略匹配器，而非按路径分割的组件序列，导致大部分嵌套忽略规则静默失效。路径现会先归一化为工作区相对组件再匹配。(`d28c2a7`)
- **已跟踪文件可能被过滤掉，而未跟踪的隐藏路径并未被忽略（与 README 描述不符）** -- `ShouldIgnoreFile` 未检查文件是否已被跟踪，导致已跟踪文件的真实变更可能被跳过；未跟踪的点前缀路径则完全未过滤。现已让已跟踪文件绕过所有忽略检查，并排除未跟踪的隐藏路径（保留显式例外）。(`0aa7a3e`)

### 环境变量与密钥

- **Git 子进程未继承父环境变量，命令错误信息泄露环境变量值** -- 传给 Git 的仅有 `repoConfig.Env` 与 `HOME`，`SSH_AUTH_SOCK`、`PATH`、`XDG_CONFIG_HOME` 和 `GIT_*` 等无法到达 Git；完整的 `Env` 切片还被嵌入命令错误信息，导致密钥和 agent socket 泄露到日志。Git 现继承完整父环境并按仓库覆盖，错误信息仅暴露变量键名而非值。(`fb3b7ec`)

### 守护进程韧性

- **单个仓库同步失败导致整个守护进程退出** -- `AutoSync` 失败时在 watcher goroutine 中调用 `log.Fatalln`，终止整个进程并中断所有其他仓库的同步，即使是短暂网络错误也会杀掉进程。同步失败现按流水线阶段分类，fetch/push 错误以有上限的退避重试，仅暂停受影响的仓库。(`3f7c00b`)

### 通知

- **变基冲突警告图标在已安装的二进制中无法加载** -- 图标通过相对路径 `assets/warning.png` 引用，安装到源码检出目录之外的二进制找不到它。图标现通过 `go:embed` 内嵌，并以 PNG 字节直接传给通知器。(`699fd7b`)

## 许可

Apache-2.0
