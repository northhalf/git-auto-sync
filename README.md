<div align="center">
  <div style="width:200px">
    <a href="https://github.com/northhalf/git-auto-sync">
      <img src="assets/icon.webp" alt="Git Auto Sync" width="200">
    </a>
  </div>

<h1>Git Auto Sync</h1>

![Status](https://img.shields.io/badge/status-active-brightgreen) ![Stage](https://img.shields.io/badge/stage-beta-blue) ![Build Status](https://github.com/northhalf/git-auto-sync/actions/workflows/ci.yml/badge.svg) ![Release](https://img.shields.io/github/v/release/northhalf/git-auto-sync) ![Downloads](https://img.shields.io/github/downloads/northhalf/git-auto-sync/total) ![License](https://img.shields.io/badge/license-Apache--2.0-blue)

<p align="center">English | <a href="./README_zh.md">中文</a></p>

<h5>A lightweight command-line tool to automatically commit and sync Git repositories.</h5>

Git Auto Sync is a modified version of <a href="https://github.com/GitJournal/git-auto-sync"><b>GitJournal/git-auto-sync</b></a>, focused on fixing bugs and improving functionality.

</div>

## Use case

Git Auto Sync is built for personal repositories where keeping a working copy in sync with its upstream matters more than a tidy commit history. It commits local changes with an auto-generated message, rebases onto the upstream branch, and pushes, so an editor or note-taking workflow stays mirrored across machines without manual commits. It is **not** a good fit for shared repositories where reviewers rely on meaningful, human-written commit messages.

## Quick Start

### Prerequisites

- Go 1.25 or newer
- Git
- A configured Git identity (`user.name` and `user.email`) for commits

### Build

```bash
git clone https://github.com/northhalf/git-auto-sync.git
cd git-auto-sync
make
```

Both `git-auto-sync` and `git-auto-sync-daemon` are built into `./bin`.

### Manual sync

Run inside any Git repository:

```bash
/path/to/bin/git-auto-sync sync
```

This commits eligible changes, fetches all remotes, rebases onto the configured upstream branch, and pushes.

### Background daemon

Register a repository for continuous monitoring:

```bash
/path/to/bin/git-auto-sync daemon add /path/to/repo
```

Check status:

```bash
/path/to/bin/git-auto-sync daemon status
```

The daemon watches the filesystem, polls every configured interval, and syncs automatically.

## Usage

Git Auto Sync provides two modes:

- **Manual**: `git-auto-sync sync` runs the sync pipeline once.
- **Daemon**: `git-auto-sync daemon add <repo>` starts a background service that monitors the repository. `daemon run`, `daemon stop`, `daemon restart`, and `daemon uninstall` control the service lifecycle.
- **Settings**: `git-auto-sync config <key> [value]` gets, sets, or unsets `syncInterval`, `debounce`, and `gitexec` at `--global` (default) or `--local` scope.

Run `git-auto-sync --help` or `git-auto-sync daemon --help` for all commands.

### Repository configuration

Settings live at two scopes: global (in `~/.config/git-auto-sync/config.json`) and per-repository
(in the Git config section `[auto-sync]`). Repository settings override global settings, which
override defaults. Time units are minutes.

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

### Nested repositories

Nested Git repositories found inside the worktree are detected and skipped, so they are never staged or committed as embedded gitlinks (mode `160000`). This applies to any nested repository, including linked worktrees created under `.claude/worktrees/`. Changes inside a nested repository belong to that repository, not the one being synchronized.

### Git and Git LFS

Git Auto Sync uses go-git only for read-only repository inspection (discovery, HEAD and branch configuration, ignore matching, and author validation). Every mutating and network operation - `status`, `add`, `commit`, `fetch`, `rebase`, and `push` - shells out to the `git` executable, resolved through `PATH` or the `gitexec` setting. A working `git` is therefore required.

If your repository uses Git LFS, install the `git-lfs` extension yourself so Git's clean and smudge filters run. Git Auto Sync detects the case where `git status` reports an LFS pointer as modified but `git add` stages nothing (for example a pointer-only working tree under `GIT_LFS_SKIP_SMUDGE`) and skips cleanly instead of failing the commit, but it does not manage LFS objects.

### Log rotation limitation

The CLI and daemon use separate rotating log files. Multiple CLI processes, such as a running `watch` command and a manual `sync`, still write to the same `git-auto-sync.log` file. The rotation library does not coordinate across processes, so concurrent CLI processes may exceed the configured rotation size or lose log records during rotation.

## Changes from the original project

Git Auto Sync is based on [GitJournal/git-auto-sync](https://github.com/GitJournal/git-auto-sync). The original project's commits up to and including `50cb029` are the baseline; everything since is this project's own work. Notable changes:

- **Engine modernization** - migrated from `src-d/go-git.v4` to `go-git/go-git/v5`, modernized the Go toolchain and dependencies, and reorganized shared code into focused `internal/` packages.
- **Git CLI for mutations** - `status`, `add`, `commit`, `fetch`, `rebase`, and `push` now run through the `git` executable instead of go-git, so content filters (including Git LFS) behave correctly. Nested repositories are detected and skipped instead of being staged as gitlinks.
- **Smarter sync** - resolves the HEAD-versus-upstream state to skip redundant rebases and pushes (equal, local-ahead, upstream-ahead, or diverged).
- **Repo-state guarding** - pauses before any mutation when the repository has an operation in progress, a detached HEAD, or no upstream, instead of failing mid-sync.
- **Watcher hardening** - debounces file changes without delaying scheduled syncs, isolates per-repository failures with capped retry backoff for remote errors, and forwards Linux and Windows wake events so a resumed machine syncs immediately.
- **Unified configuration** - a `config` CLI and config-file polling reload global and per-repository settings (`syncInterval`, `debounce`, `gitexec`) without restarting the daemon.
- **Improved ignore rules** - tracked files are always eligible, untracked hidden paths are ignored with explicit exceptions (`.github/`, Git control files, `*.example`), and ignore matching normalizes paths and caches the index per sync round.
- **Daemon and CLI UX** - `daemon run`, `stop`, `restart`, and `uninstall` commands; per-repository runtime status and last-synced time in `ls` and `status`; structured rotating logs for CLI and daemon; full parent-environment inheritance with secret redaction; and Windows service fixes so LocalSystem shares the user's paths and Git config.

## License

Apache-2.0
