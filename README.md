<div align="center">
  <div style="width:200px">
    <a href="https://github.com/northhalf/git-auto-sync">
      <img src="assets/icon.webp" alt="Git Auto Sync" width="200">
    </a>
  </div>

<h1>Git Auto Sync</h1>

![Status](https://img.shields.io/badge/status-active-brightgreen) ![Stage](https://img.shields.io/badge/stage-stable-firebrick) ![Build Status](https://github.com/northhalf/git-auto-sync/actions/workflows/ci.yml/badge.svg) ![Release](https://img.shields.io/github/v/release/northhalf/git-auto-sync) ![Downloads](https://img.shields.io/github/downloads/northhalf/git-auto-sync/total) ![License](https://img.shields.io/badge/license-Apache--2.0-blue)

<p align="center">English | <a href="./README_zh.md">中文</a></p>

<h5>A lightweight command-line tool to automatically commit and sync Git repositories.</h5>

Git Auto Sync is a modified version of <a href="https://github.com/GitJournal/git-auto-sync"><b>GitJournal/git-auto-sync</b></a>, focused on fixing bugs and improving functionality.

</div>

## Use case

Git Auto Sync is built for personal repositories where keeping a working copy in sync with its upstream matters more than a tidy commit history. It commits local changes with an auto-generated message, rebases onto the upstream branch, and pushes, so an editor or note-taking workflow stays mirrored across machines without manual commits. It is **not** a good fit for shared repositories where reviewers rely on meaningful, human-written commit messages.

## Quick Start

### Prerequisites

- Git
- A configured Git identity (`user.name` and `user.email`) for commits
- Building from source additionally requires Go 1.25 or newer

### Get the binaries

Choose one of the following two methods.

<details>
<summary><b>Option A - Download a release</b></summary>

Download the archive for your platform from the [releases page](https://github.com/northhalf/git-auto-sync/releases/latest) and extract it. Each archive contains the two binaries (`git-auto-sync`, `git-auto-sync-daemon`; with `.exe` on Windows) and a `completions/` folder with the shell completion scripts.

```bash
# Example: extract a Linux x86_64 release into ~/.local/share/git-auto-sync
mkdir -p ~/.local/share/git-auto-sync
tar -xzf git-auto-sync_*_Linux_x86_64.tar.gz -C ~/.local/share/git-auto-sync
```

On Linux and macOS, make both extracted binaries executable before running them:

```bash
chmod +x /path/to/binaries/git-auto-sync /path/to/binaries/git-auto-sync-daemon
```

Without the executable permission, the shell cannot run the files even if their directory is on `PATH`.

</details>

<details>
<summary><b>Option B - Build from source</b></summary>

```bash
git clone https://github.com/northhalf/git-auto-sync.git
cd git-auto-sync
make
```

Both `git-auto-sync` and `git-auto-sync-daemon` are built into `./bin`. Completion scripts live in `completions/`.

</details>

### Add the program directory to your PATH

Whichever method you used, put the directory holding the binaries on your `PATH` so you can invoke `git-auto-sync` directly:

```bash
# Linux / macOS - add to ~/.bashrc or ~/.zshrc
export PATH="$PATH:/path/to/binaries"

# Windows (PowerShell) - set for the current user
[Environment]::SetEnvironmentVariable("PATH", $env:PATH + ";C:\path\to\binaries", "User")
```

### Shell completion (optional)

The `completions/` folder ships scripts for bash, zsh, and PowerShell. Source the one for your shell (replace `/path/to/completions/` with the actual path - the `completions/` folder from the release archive, or `completions/` in the cloned repo):

```bash
# bash - add to ~/.bashrc. Requires the bash-completion package.
source /path/to/completions/bash_autocomplete

# zsh - add to ~/.zshrc, or drop the file into a directory on your $fpath
source /path/to/completions/zsh_autocomplete

# PowerShell - dot-source from your profile
. C:\path\to\completions\powershell_autocomplete.ps1
```

### Manual sync

Run inside any Git repository:

```bash
git-auto-sync sync
```

This commits eligible changes, fetches all remotes, rebases onto the configured upstream branch, and pushes.

### Background daemon

Register a repository for continuous monitoring:

```bash
git-auto-sync daemon add /path/to/repo
```

Check status:

```bash
git-auto-sync daemon status
```

The daemon watches the filesystem, polls every configured interval, and syncs automatically.

## Usage

Git Auto Sync provides two modes:

- **Manual**: `git-auto-sync sync` runs the sync pipeline once.
- **Daemon**: `git-auto-sync daemon add <repo>` starts a background service that monitors the repository. `daemon run`, `daemon stop`, `daemon restart`, and `daemon uninstall` control the service lifecycle.
- **Settings**: `git-auto-sync config <key> [value]` gets, sets, or unsets `syncInterval`, `debounce`, and `gitexec` at `--global` (default) or `--local` scope.

Run `git-auto-sync --help` or `git-auto-sync daemon --help` for all commands.

### Repository configuration

Settings live at two scopes: global (in the platform config file, e.g.
`~/.config/git-auto-sync/config.json` on Linux - see [File locations](#file-locations)) and
per-repository (in the Git config section `[auto-sync]`). Repository settings override global
settings, which override defaults. Time units are minutes.

```bash
git-auto-sync config syncInterval 60          # minutes, default 60 (global)
git-auto-sync config --local syncInterval 30  # per-repo override
git-auto-sync config --local debounce 5       # minutes, default 10
git-auto-sync config --global gitexec /usr/bin/git  # default: git from PATH
git-auto-sync config --list                   # show effective settings
git-auto-sync config --unset syncInterval     # remove a setting (default: global)
```

Defaults: sync once per hour, ten-minute debounce, `git` resolved through `PATH`.

### Debounce

The watcher debounces filesystem and wake events using the `debounce` setting (default 10 minutes). Each event resets a timer, and a sync runs only after the configured period elapses with no further events, so a burst of edits coalesces into a single commit rather than one commit per save. Periodic ticks from `syncInterval` and machine-wake events bypass the debounce and trigger a sync immediately, so scheduled and resume syncs are never delayed. Triggers that arrive while a sync is already running are coalesced into one follow-up sync after it finishes.

### Commit messages

Every commit message is generated from `git status --porcelain`. Each eligible change becomes one line in the form `XY path`, where `XY` is the two-character Git status code (for example `M`, `A`, or `??`) and `path` is the repository-relative path. The lines are sorted alphabetically and joined with newlines, then passed to `git commit -m`:

```
?? notes/2026-07-18.md
M src/main.go
A docs/changelog.md
```

There is no human-written summary. This is why the tool suits workflows that prioritize staying in sync over a readable history (see [Use case](#use-case)).

### When sync pauses

Git Auto Sync uses rebase, not merge. Several conditions stop the watcher from syncing a repository until you intervene. When any of them is detected, it sends a desktop notification and pauses that repository; recovery requires fixing the condition and then **restarting the daemon** (or removing and re-adding the repository):

- **A Git operation is in progress** - an unfinished merge, rebase, cherry-pick, or revert.
- **Detached HEAD** - HEAD is not on a branch, so there is nothing to rebase onto or push.
- **No upstream** - the current branch has no configured upstream tracking branch.
- **Missing Git identity** - `user.name` or `user.email` is not set.
- **Rebase conflict** - a rebase onto the upstream conflicts; the rebase is aborted and the repository pauses before push.

Network errors from `fetch` and `push` do **not** pause the repository. The watcher retries them with capped backoff (2, 4, 8, 15, 30, then 60 minutes) and resumes automatically once the remote is reachable again.

### Ignored files

Files already tracked by Git are always synced and bypass every ignore rule. For untracked paths, any path with a dot-prefixed component is excluded from commits and filesystem monitoring, unless it is `.github/` content, a Git control file (`.gitignore`, `.gitattributes`, `.gitmodules`, `.gitkeep`, or `.mailmap`) at any depth, or a file whose name ends in `.example`. Empty files(other than `.gitkeep`), Git-ignored files, Git metadata, and editor swap/backup files (e.g., Vim, Emacs) stay excluded even when an exception applies.

If you want a path that is excluded by default (for example a dotfile that is not a Git control file) to be synced, stage it yourself with `git add`. Once Git tracks the file it is always eligible and bypasses every ignore rule above.

In addition to the dot-prefix convention, OS-level hidden attributes are honored on platforms that have them, with no name exceptions:

- **Windows** - a file or any ancestor directory carrying the `FILE_ATTRIBUTE_HIDDEN` attribute (set through File Explorer properties or `attrib +H`) is excluded.
- **macOS** - a file or any ancestor directory carrying the `UF_HIDDEN` file flag (set with `chflags hidden`) is excluded.

A hidden attribute on an ancestor directory excludes every untracked path beneath it. Tracked files still bypass these checks. Linux has no equivalent filesystem attribute, so only the dot-prefix convention applies there; the GTK `.hidden` file convention is not implemented because it is desktop-specific (Nautilus/Nemo only) and name-based rather than a filesystem attribute.

### Nested repositories

Nested Git repositories found inside the worktree are detected and skipped, so they are never staged or committed as embedded gitlinks (mode `160000`). This applies to any nested repository, including linked worktrees created under `.claude/worktrees/`. Changes inside a nested repository belong to that repository, not the one being synchronized.

### Git and Git LFS

Git Auto Sync uses go-git only for read-only repository inspection (discovery, HEAD and branch configuration, ignore matching, and author validation). Every mutating and network operation - `status`, `add`, `commit`, `fetch`, `rebase`, and `push` - shells out to the `git` executable, resolved through `PATH` or the `gitexec` setting. A working `git` is therefore required.

If your repository uses Git LFS, install the `git-lfs` extension yourself so Git's clean and smudge filters run. Git Auto Sync detects the case where `git status` reports an LFS pointer as modified but `git add` stages nothing (for example a pointer-only working tree under `GIT_LFS_SKIP_SMUDGE`) and skips cleanly instead of failing the commit, but it does not manage LFS objects.

### Log rotation limitation

The CLI and daemon use separate rotating log files. Multiple CLI processes, such as a running `watch` command and a manual `sync`, still write to the same `git-auto-sync.log` file. The rotation library does not coordinate across processes, so concurrent CLI processes may exceed the configured rotation size or lose log records during rotation.

### File locations

Git Auto Sync stores configuration and logs in platform-specific directories.

**Configuration** - the global settings file holds `repos`, `envs`, `syncInterval`, `debounce`, and `gitexec`:

| Platform | Path |
| --- | --- |
| Linux | `~/.config/git-auto-sync/config.json` |
| macOS | `~/Library/Application Support/git-auto-sync/config.json` |
| Windows | `%AppData%\git-auto-sync\config.json` |

Per-repository settings are stored in the repository's own Git config under the `[auto-sync]` section, not in this file.

**Logs** - the CLI and daemon each write a rotating log file (10 MB per file, 3 backups retained):

| Platform | Directory |
| --- | --- |
| Linux | `~/.local/share/git-auto-sync/log/` |
| macOS | `~/Library/Logs/` |
| Windows | `%LOCALAPPDATA%\git-auto-sync\logs\` |

| File | Writer |
| --- | --- |
| `git-auto-sync.log` | `git-auto-sync` CLI |
| `git-auto-sync-daemon.log` | daemon service |

See [Log rotation limitation](#log-rotation-limitation) for caveats about concurrent CLI processes sharing the same log file.

### Windows permissions

The daemon runs as a Windows service installed under the `LocalSystem` account. Subcommands that manage that service - `daemon run`, `stop`, `restart`, and `uninstall` - go through the Windows Service Control Manager and require an **administrator** terminal. In a non-elevated terminal they fail with `Access is denied`.

Open the terminal as administrator before using these `daemon` subcommands. As a convenience, Windows Terminal can launch a profile as administrator by default: open the profile dropdown, choose **Settings**, select the target profile, and turn on **Run this profile as Administrator**:

![Windows Terminal: set a profile to run as administrator by default](assets/windows-terminal-admin.webp)

## Changes from the original project

Git Auto Sync is based on [GitJournal/git-auto-sync](https://github.com/GitJournal/git-auto-sync). The original project's commits up to and including `50cb029` are the baseline; everything since is this project's own work. Notable changes:

- **Engine modernization** - migrated from `src-d/go-git.v4` to `go-git/go-git/v5`, modernized the Go toolchain and dependencies, and reorganized shared code into focused `internal/` packages.
- **Git CLI for mutations** - `status`, `add`, `commit`, `fetch`, `rebase`, and `push` now run through the `git` executable instead of go-git, so content filters (including Git LFS) behave correctly. Nested repositories are detected and skipped instead of being staged as gitlinks.
- **Smarter sync** - resolves the HEAD-versus-upstream state to skip redundant rebases and pushes (equal, local-ahead, upstream-ahead, or diverged).
- **Repo-state guarding** - pauses before any mutation when the repository has an operation in progress, a detached HEAD, or no upstream, instead of failing mid-sync.
- **Watcher hardening** - debounces file changes without delaying scheduled syncs, isolates per-repository failures with capped retry backoff for remote errors, and forwards Linux and Windows wake events so a resumed machine syncs immediately.
- **Unified configuration** - a `config` CLI and config-file polling reload global and per-repository settings (`syncInterval`, `debounce`, `gitexec`) without restarting the daemon.
- **Improved ignore rules** - tracked files are always eligible, untracked hidden paths are ignored with explicit exceptions (`.github/`, Git control files, `*.example`), and ignore matching normalizes paths and caches the index per sync round.
- **Daemon and CLI UX** - `daemon run`, `stop`, `restart`, and `uninstall` commands; structured rotating logs for CLI and daemon; full parent-environment inheritance with secret redaction; and Windows service fixes so LocalSystem shares the user's paths and Git config.
- **Monitoring list visualization** - unlike the original project, `daemon ls` and `daemon status` render every monitored repository as an aligned table: a live runtime status (`running`, `paused (<reason>)`, or `unknown (daemon may not be running)`) and a last-synced time (`never synced`, or a relative duration such as `synced 3m ago`), beneath a header reporting the daemon service state. Which repositories are healthy, paused, or stale is visible at a glance.

## Bug fixes from the original project

Beyond the improvements listed above, the following defects present in the original `GitJournal/git-auto-sync` baseline (commit `50cb029`) have been fixed:

### Windows service

- **The daemon service failed to start on Windows** - the service executable path was registered without an `.exe` suffix, which Windows does not append when launching services, so the service could not start. The path now carries the platform-correct suffix. (`2f781ef`)
- **The Windows daemon ran as LocalSystem and lost the user's paths and Git config** - installed services run as `LocalSystem`, whose blank profile left the daemon unable to write logs, resolve repositories, find `git` on `PATH`, read the user's Git identity, or operate the user's worktrees (`dubious-ownership`). The installer now injects the user's `APPDATA`, `LOCALAPPDATA`, `USERPROFILE`, and `Path`, and passes `-c safe.directory=<repo>` per repository on Windows. (`6c21dbf`)

### Commits and Git LFS

- **Unchanged Git LFS files were re-committed on every sync, and `check` crashed on empty or root paths** - go-git's status and staging bypass Git's clean/smudge filters, so LFS-tracked files with unchanged pointers read as modified and were committed every cycle; `ShouldIgnoreFile` also panicked on empty or repository-root paths. LFS pointers are now skipped and path validation rejects empty and root paths. (`906d831`)
- **Nested Git repositories were staged as embedded gitlinks, and linked worktrees were recursed into** - any untracked, non-ignored path was staged through go-git, including nested repositories (committed as mode-`160000` gitlinks) and `git worktree` directories. The commit stage now uses `git status --porcelain` and `git add`, detecting and skipping nested repositories. (`d3c6e0f`)

### Ignore rules

- **Git ignore rules for nested or absolute paths did not apply** - full paths were handed to go-git's ignore matcher as a single component rather than split segments, so most nested ignore patterns silently failed. Paths are now normalized to worktree-relative segments before matching. (`d28c2a7`)
- **Tracked files could be filtered out, and untracked hidden paths were not ignored despite the README claiming otherwise** - `ShouldIgnoreFile` did not check whether a file was tracked, so real changes to tracked files could be skipped; untracked dot-prefixed paths were not filtered at all. Tracked files now bypass every ignore check, and untracked hidden paths are excluded with explicit exceptions. (`0aa7a3e`)

### Environment and secrets

- **Git subprocesses did not inherit the parent environment, and command errors leaked environment values** - only `repoConfig.Env` plus `HOME` reached Git, so `SSH_AUTH_SOCK`, `PATH`, `XDG_CONFIG_HOME`, and `GIT_*` never did; full `Env` slices were also embedded in command error messages, leaking secrets and agent sockets into logs. Git now inherits the full parent environment with per-repository overrides, and errors expose only variable keys, not values. (`fb3b7ec`)

### Daemon resilience

- **A single repository's sync failure killed the entire daemon** - `AutoSync` failures called `log.Fatalln` in the watcher goroutine, terminating the process and interrupting every other repository, even on transient network errors. Sync failures are now classified by pipeline stage, fetch/push errors retry with capped backoff, and only the affected repository pauses. (`3f7c00b`)

### Notifications

- **The rebase-conflict warning icon failed to load in installed binaries** - the icon was referenced by the relative path `assets/warning.png`, which binaries installed outside the source checkout could not find. The icon is now embedded with `go:embed` and passed to the notifier as PNG bytes. (`699fd7b`)

## License

Apache-2.0
