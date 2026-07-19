# Android/Termux 支持设计

日期：2026-07-19

## 目标

为 `git-auto-sync` 增加一等 Android/Termux 支持，解决普通 `linux/arm64` Go 二进制在 Android seccomp 环境中触发 `faccessat2`/`SIGSYS`，以及 Termux 缺少 systemd/SysV `service` 命令的问题。

本次范围包括：

1. 发布独立的 `android/arm64` CLI 与 daemon 二进制；
2. Android 唤醒通知使用明确的 no-op；
3. Android 系统通知通过 `termux-notification` 发送，并在命令缺失时非致命警告；
4. Android daemon 生命周期由 `termux-services`/runit 管理；
5. 保持 Linux、Windows、macOS 的现有行为不变。

本次不支持 Android ARMv7 或 Android x86_64，不自动安装 Termux 软件包或 Android 应用，也不新增自定义 PID 文件或后台轮询机制。

## 外部依赖

基础同步仍要求 Git：

```sh
pkg install git
```

使用 Android daemon 时要求：

```sh
pkg install termux-services
```

安装后用户需重启 Termux，或执行：

```sh
source "$PREFIX/etc/profile.d/start-services.sh"
```

Android 通知是可选功能。启用通知需要：

```sh
pkg install termux-api
```

用户还必须安装与当前 Termux 来源匹配的 Termux:API Android 应用。程序不会自动执行任何 `pkg install` 命令。

本次不新增 Go 第三方依赖、LSP 或格式化工具。项目继续要求 Go 1.25 或更新版本；Release CI 使用 Go 1.26。

## 平台架构

使用 Go build tags 将 Android 与普通 Linux 明确分离：

```text
android
linux && !android
darwin
windows
```

平台差异通过小型后端实现，而不是在通用逻辑中散布大量 `runtime.GOOS` 分支。

主要新增或拆分的组件为：

```text
internal/watcher/awake_notifier_android.go
internal/notification/notification.go
internal/notification/notification_android.go
internal/notification/notification_desktop.go
internal/daemonservice/backend_android.go
internal/daemonservice/backend_nonandroid.go
daemon/main_android.go
daemon/main_nonandroid.go
```

文件名可在实施时根据现有包结构微调，但职责边界不得改变。

## Android 唤醒通知

`internal/watcher/awake_notifier_android.go` 提供明确的 no-op 实现：

- `NewAwakeNotifier` 始终返回成功；
- `Start` 立即返回 `nil`；
- 不连接 D-Bus；
- 不访问 systemd-logind；
- 不创建 goroutine；
- 不向 awake channel 写入事件；
- context 取消不会产生错误。

现有 `awake_notifier_linux.go` 增加 `linux && !android` build constraint，防止 Android 隐式复用 Linux systemd-logind 行为。

Android watcher 继续依赖：

- `rjeczalik/notify` 的文件系统事件；
- 现有 debounce；
- 现有同步周期 ticker。

本次不新增唤醒轮询或 Android 电源 API。

## 通知抽象

### 公共行为

将 `beeep.Alert` 从 `internal/syncer.AutoSync` 中移出，使用内部通知包。建议接口为：

```go
notification.Alert(title, content string) error
notification.WarnIfUnavailable(logger *slog.Logger)
```

Android 和桌面平台分别实现该接口：

- Android 调用 `termux-notification`；
- Linux、macOS、Windows 继续调用 `beeep.Alert` 并使用现有 `assets.WarningPNG`。

通知失败始终是非致命的。它不能覆盖以下真实错误：

- 仓库状态暂停；
- 缺少 upstream；
- detached HEAD；
- rebase 冲突；
- 其他同步阶段错误。

### 启动可用性检查

Android 只在以下入口检查通知命令：

- `git-auto-sync sync`；
- `git-auto-sync watch`；
- `git-auto-sync daemon ...` 的所有子命令；
- `git-auto-sync-daemon` 进程启动。

以下入口不检查：

- `config`；
- `check`；
- `--help`；
- `--version`。

检查结果在每个进程内缓存，只执行一次，不新增周期检查或后台 goroutine。

Android 首先检查 `$PREFIX/bin/termux-notification`，必要时再使用 `exec.LookPath("termux-notification")`。实际发送时使用解析出的绝对路径。

命令缺失时同时：

1. 向 stderr 写入警告；
2. 使用当前 logger 写一条 `Warn`；
3. 继续执行当前命令。

警告内容必须包含：

```text
warning: Android notifications are unavailable because termux-notification was not found.
Install the Termux:API Android app, then run:
pkg install termux-api
```

通知包提供可判定的 `ErrUnavailable`。启动检查已经警告后，`AutoSync` 遇到该错误不得重复写同一缺失警告；其他通知执行错误继续记录。

### Android 通知执行

Android 使用独立参数调用，不经过 shell：

```text
termux-notification --title <title> --content <content>
```

不使用固定 `--id`，避免暂停通知与冲突通知互相覆盖。Android 不传递 PNG 图标。

通知命令使用 10 秒超时：

- 正常退出表示成功；
- 超时取消子进程并返回错误；
- 命令存在但 Termux:API Android 应用缺失或不可访问时，返回带命令输出的错误；
- 错误由调用方记录，但不替代原始同步错误。

启动检查只验证命令存在，不发送测试通知，因此不会在每次启动时制造 Android 通知。

## daemonservice 抽象

通用 `Service` 不再要求调用方直接依赖具体的 `kardianos/service.Service` 字段，而是依赖最小生命周期后端：

```go
type backend interface {
    Status() (service.Status, error)
    Install() error
    Uninstall() error
    Start() error
    Stop() error
}
```

具体接口可额外包含通用实现真正需要的方法，但不得要求 Android runit 后端伪造 `Logger`、`SystemLogger` 或 service-hosting `Run` 等无关能力。

- 非 Android 后端包装现有 `kardianos/service`；
- Android 后端控制 `termux-services`/runit；
- 现有 `Enable`、`EnsureRunning`、`Stop`、`Restart`、`Disable`、`Status` 的用户语义保持一致；
- 非 Android 的安装、日志与服务管理行为保持不变。

## Android runit 后端

### 路径

Android 后端只依据 `$PREFIX`，不硬编码 Termux 包名或 `/data/data/...` 路径：

```text
sv executable:       $PREFIX/bin/sv
service root:        $PREFIX/var/service
service directory:   $PREFIX/var/service/git-auto-sync-daemon
logger executable:   $PREFIX/share/termux-services/svlogger
```

构造或安装时验证：

1. `$PREFIX` 非空且为绝对路径；
2. `$PREFIX/bin/sv` 存在且可执行；
3. `$PREFIX/var/service` 存在；
4. `$PREFIX/share/termux-services/svlogger` 存在；
5. CLI 同目录中的 `git-auto-sync-daemon` 存在且可执行。

缺少 termux-services 时返回包含以下操作建议的错误：

```text
Termux daemon management requires termux-services.
Install it with: pkg install termux-services
Then restart Termux or run:
source "$PREFIX/etc/profile.d/start-services.sh"
```

所有 `sv` 调用使用绝对可执行文件路径和绝对服务目录路径，不依赖 `PATH`。

### 受管服务目录

程序创建：

```text
$PREFIX/var/service/git-auto-sync-daemon/
├── .git-auto-sync-managed
├── run
└── log/
    └── run -> $PREFIX/share/termux-services/svlogger
```

`.git-auto-sync-managed` 的内容固定为：

```text
git-auto-sync-runit-v1
```

只有内容完全匹配（允许末尾单个换行）的标记才证明目录由本程序管理。未知版本或其他内容均按未受管目录处理。

`run` 脚本语义为：

```sh
#!$PREFIX/bin/sh
exec 2>&1
exec "/absolute/path/to/git-auto-sync-daemon"
```

实施时必须使用 POSIX 单引号转义安全处理可执行文件路径，不能将未经处理的值拼接为 shell 代码。`run` 权限为 `0755`。新安装先在服务根目录下创建临时目录，完整写入标记、脚本和日志链接后再原子重命名为正式服务目录；更新已有受管服务时，各普通文件使用临时文件和原子重命名，避免中断后留下半写内容。

服务目录处理规则：

1. 目录不存在：创建受管服务；
2. 目录存在且标记有效：允许更新受管脚本和 logger 配置；
3. 目录存在但无有效标记：拒绝覆盖；
4. 不自动备份、重命名或删除用户自定义服务。

拒绝错误必须指出冲突目录，例如：

```text
refusing to overwrite unmanaged runit service: <service-directory>
```

### 生命周期映射

| 通用操作 | Android/runit 行为 |
|---|---|
| `Install` | 创建或更新受管服务目录 |
| `Start` | 执行 `$PREFIX/bin/sv up <absolute-service-directory>` |
| `Stop` | 执行 `$PREFIX/bin/sv down <absolute-service-directory>` |
| `Restart` | 使用通用 `Stop` 后 `Start` 流程，不单独调用 `sv restart` |
| `Status` | 验证受管目录后执行 `sv status` |
| `Uninstall` | 停止服务并删除受管服务目录 |

`Status` 的处理规则：

- 服务目录不存在：返回 `ErrNotInstalled`；
- 服务目录不受管：返回 ownership 错误；
- `sv status` 输出以 `run:` 开头：`service.StatusRunning`；
- 输出以 `down:` 开头：`service.StatusStopped`；
- supervise 不可用、命令异常或输出未知：`service.StatusUnknown` 加具体错误。

termux-services 尚未启动时不得误报为 daemon stopped，错误应建议用户重启 Termux 或 source `start-services.sh`。

### 卸载

Android `daemon uninstall`：

1. 验证所有权标记；
2. 运行中的服务先执行 `sv down`；
3. 只有停止成功后才删除 `$PREFIX/var/service/git-auto-sync-daemon`；
4. `sv down` 失败时返回错误并完整保留服务目录；
5. 不删除其他路径。

必须保留：

- `$PREFIX/var/log/sv/git-auto-sync-daemon`；
- 应用轮转日志；
- `config.json`；
- `state.json`；
- daemon 仓库列表；
- 仓库内容和 Git 配置。

未受管目录不得由 uninstall 删除。

## Android daemon 进程入口

将 daemon hosting 入口与生命周期控制分离：

- 非 Android 继续通过 `kardianos/service.Run()` 托管；
- Android 设置 daemon logger、执行通知可用性检查后，直接以前台进程运行现有 daemon 主循环；
- runit 负责启动、监督、停止和重启；
- Android daemon 不 fork；
- 不创建自定义 PID 文件；
- 不调用 `service`、systemctl 或其他 Linux init 命令。

建议拆分：

```text
daemon/main_android.go
daemon/main_nonandroid.go
```

共享 watcher manager、配置轮询和 daemon 主循环。

## Release

在 `.goreleaser.yaml` 新增两个独立构建：

```yaml
- id: git-auto-sync-android
  binary: git-auto-sync
  env:
    - CGO_ENABLED=0
    - RELEASE=true
  goos:
    - android
  goarch:
    - arm64

- id: git-auto-sync-daemon-android
  binary: git-auto-sync-daemon
  dir: daemon
  env:
    - CGO_ENABLED=0
    - RELEASE=true
  goos:
    - android
  goarch:
    - arm64
```

Android 归档包含两个二进制以及现有文档、许可证和 shell completions。预期归档名：

```text
git-auto-sync_vX.Y.Z_Android_arm64.tar.gz
```

Linux、Windows、macOS 的现有目标和归档格式不得改变。

## 测试策略

### 常规测试

```sh
go test ./...
```

### Android 交叉构建

```sh
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build ./...
```

### 格式检查

```sh
test -z "$(gofmt -l -- *.go daemon/*.go internal/config/*.go internal/daemonstate/*.go internal/daemonservice/*.go internal/logging/*.go internal/notification/*.go internal/syncer/*.go internal/watcher/*.go)"
```

### Lint

```sh
golangci-lint run ./...
```

复现 CI lint 的安装命令：

```sh
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b "$(go env GOPATH)/bin" v2.12.2
```

### Release 配置

```sh
goreleaser check
```

### runit 后端测试

平台构造由 Android build-tag 文件负责；路径验证、脚本生成、标记校验、状态解析和命令执行逻辑应可注入并在普通 CI 主机运行测试。

至少覆盖：

1. 缺少或非法 `$PREFIX`；
2. 缺少 `sv` 或服务根目录；
3. 创建受管服务；
4. 更新已有受管服务；
5. 拒绝覆盖无标记服务；
6. 脚本权限和路径转义；
7. logger symlink；
8. running/stopped/unknown 状态解析；
9. supervise 不可用；
10. uninstall 保留日志和应用配置；
11. uninstall 拒绝删除无标记服务。

### 通知测试

至少覆盖：

1. 仅 sync/watch/daemon 检查；
2. 命令存在时不警告；
3. 命令缺失时 stderr 和 Warn 日志均出现；
4. 每个进程只警告一次；
5. title/content 作为独立参数传递；
6. 特殊字符不进入 shell；
7. 10 秒超时取消命令；
8. `ErrUnavailable` 不覆盖同步错误；
9. 其他发送错误被记录；
10. 非 Android 继续使用 beeep。

## 文档

更新 `README.md` 和 `README_zh.md`，新增 Android/Termux 章节，说明：

- 下载 `Android_arm64`，不能使用 `Linux_arm64`；
- daemon 依赖 `termux-services`；
- 通知依赖 Termux:API 应用和 `termux-api` 包，但属于可选功能；
- Android 唤醒通知是 no-op；
- watcher 仍依赖文件事件和同步 ticker；
- Android 12+ 可能清理 Termux 后台进程；
- `termux-wake-lock` 可选且可能增加耗电；
- daemon uninstall 保留配置、状态、仓库列表和日志。

## 验收标准

### Android 构建

`go version -m` 显示：

```text
GOOS=android
GOARCH=arm64
CGO_ENABLED=0
```

`git-auto-sync sync` 和 `watch` 不再出现 `faccessat2`/`SIGSYS`，也不尝试连接 systemd-logind。

### 通知

- 未安装 `termux-notification` 时，sync/watch/daemon 启动警告一次并继续；
- config/check/help/version 不警告；
- 安装后，暂停和 rebase 冲突产生 Android 系统通知；
- 通知错误不改变真实同步错误。

### daemon

以下命令通过 runit 工作且不执行 Linux `service`：

```sh
git-auto-sync daemon add /path/to/repo
git-auto-sync daemon status
git-auto-sync daemon stop
git-auto-sync daemon run
git-auto-sync daemon restart
git-auto-sync daemon uninstall
```

服务状态正确，非受管服务不被覆盖或删除，uninstall 保留日志和应用数据。

### 非 Android 回归

- Linux、Windows、macOS Release 目标不减少；
- 非 Android daemon 继续使用 kardianos/service；
- 非 Android 通知继续使用 beeep；
- Linux systemd-logind 唤醒行为不变；
- 全部现有测试通过。
