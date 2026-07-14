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
- **Daemon**: `git-auto-sync daemon add <repo>` starts a background service that monitors the repository.

Run `git-auto-sync --help` or `git-auto-sync daemon --help` for all commands.

### Repository configuration

Per-repository settings live in the Git config section `[auto-sync]`:

```bash
git config --local auto-sync.syncInterval 300   # seconds, default 600
git config --local auto-sync.exec /path/to/git  # optional custom git executable
```

### Merge conflicts

Git Auto Sync uses rebase, not merge. If a rebase conflict occurs, it aborts the rebase, sends a desktop notification, and stops syncing that repository until the conflict is resolved.

### Ignored files

Hidden files, files ignored by Git, and editor swap/backup files (e.g., Vim, Emacs) are excluded from commits and filesystem monitoring.

## License

Apache-2.0
