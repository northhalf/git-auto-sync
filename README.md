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
- **Daemon**: `git-auto-sync daemon add <repo>` starts a background service that monitors the repository. `daemon run`, `daemon stop`, and `daemon uninstall` control the service lifecycle.
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

### Merge conflicts

Git Auto Sync uses rebase, not merge. If a rebase conflict occurs, it aborts the rebase, sends a desktop notification, and stops syncing that repository until the conflict is resolved.

### Ignored files

Files already tracked by Git are always synced and bypass every ignore rule. For untracked paths, any path with a dot-prefixed component is excluded from commits and filesystem monitoring, unless it is `.github/` content, a Git control file (`.gitignore`, `.gitattributes`, `.gitmodules`, `.gitkeep`, or `.mailmap`) at any depth, or a file whose name ends in `.example`. Empty files(other than `.gitkeep`), Git-ignored files, Git metadata, and editor swap/backup files (e.g., Vim, Emacs) stay excluded even when an exception applies.

### Nested repositories

Nested Git repositories found inside the worktree are detected and skipped, so they are never staged or committed as embedded gitlinks (mode `160000`). This applies to any nested repository, including linked worktrees created under `.claude/worktrees/`. Changes inside a nested repository belong to that repository, not the one being synchronized.

### Log rotation limitation

The CLI and daemon use separate rotating log files. Multiple CLI processes, such as a running `watch` command and a manual `sync`, still write to the same `git-auto-sync.log` file. The rotation library does not coordinate across processes, so concurrent CLI processes may exceed the configured rotation size or lose log records during rotation.

## License

Apache-2.0
