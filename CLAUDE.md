# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Prerequisites

- Go 1.25 or newer, as declared in `go.mod`.
- The `git` executable. Sync operations shell out to Git, and repository tests create commits and local remotes.
- A Git identity for tests that create commits:

  ```bash
  git config --global user.email "test@example.com"
  git config --global user.name "Git Auto Sync Tests"
  ```

- CI uses golangci-lint v2.12.2. Install the same version when reproducing its lint job:

  ```bash
  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v2.12.2
  ```

## Build, test, and lint

Run commands from the repository root unless a command says otherwise.

```bash
# Build both executables into ./bin
make

# Run the full test suite
go test ./...

# Run one package
go test ./common

# Run one test by exact name
go test ./common -run '^Test_NewFile$'

# Disable the test cache while iterating
go test -count=1 ./common -run '^Test_RebaseBothCommitsConflict$'

# Run the CI linter
golangci-lint run ./...

# Check formatting without modifying files
test -z "$(gofmt -l -- *.go common/*.go common/config/*.go daemon/*.go)"

# Apply formatting
gofmt -w -- *.go common/*.go common/config/*.go daemon/*.go
```

The integration-style tests under `common` copy repositories from `common/testdata` into temporary directories. Fixtures store repository metadata as `.gitted`; test helpers rename it to `.git` after copying. Fetch and rebase fixtures also rewrite `$TESTDATA$` remote paths. Preserve that layout when adding fixture cases.

## Architecture

The project produces two executables:

- The root `main` package builds `git-auto-sync`, the user-facing CLI. `main.go` defines `watch`, `sync`, `check`, and `daemon` commands. `daemon.go` implements daemon configuration and service-management subcommands.
- `daemon/main.go` builds `git-auto-sync-daemon`, the service process. It reads the configured repositories, starts one watcher goroutine per repository, and runs through `kardianos/service`. The service setup expects this binary beside `git-auto-sync`.

Most behavior lives in `common`:

1. `NewRepoConfig` reads per-repository settings from the Git config section `[auto-sync]`. `syncInterval` controls periodic syncs in seconds, and `exec` selects a Git executable. The CLI can append explicit `--env KEY=VALUE` entries for a manual sync.
2. `AutoSync` runs a fixed pipeline: verify `user.email` and `user.name`, commit eligible worktree changes, fetch every remote, rebase onto the current branch's configured upstream, then push to that upstream.
3. The code uses `go-git` for repository discovery, status, staging, branch configuration, and ignore matching. Mutating/network operations run the external `git` command through `GitCommand`, which controls the working directory and environment.
4. A rebase conflict triggers `git rebase --abort`, returns `errRebaseFailed`, and causes `AutoSync` to send a desktop notification. The sync stops before push.
5. `WatchForChanges` performs an initial sync, then combines recursive filesystem notifications, a periodic ticker, and platform-specific wake notifications. It filters events through `ShouldIgnoreFile` before requesting another sync.

Daemon state is separate from repository-local settings. `common/config` stores watched repository paths and daemon environment entries in the platform-local `git-auto-sync/config.json`. The CLI updates this file and installs, starts, stops, or uninstalls the user service through `common/service.go`.

`ShouldIgnoreFile` is shared by the watcher and commit path. It excludes editor swap/backup files, `.git` contents, empty files, and files matched by Git ignore/exclude rules. Keep event filtering and commit filtering aligned when changing ignore behavior.

Platform wake behavior relies on Go filename suffixes: Darwin uses `mac-sleep-notifier`; Linux and Windows currently provide no-op implementations. Release builds enable CGO for Darwin because the filesystem notification dependency requires it, while Linux and Windows builds disable CGO.

## Code style

All functions — especially exported ones — **must** include structured doc comments using the following tag‑based style:

- `@description` – a brief, clear explanation of what the function does, including side effects or important behavior.
- `@param` – for each parameter: name, and a short explanation (e.g., "path to repository root").
- `@return` – for each return value: name (if any), and a description of what it represents or when it is returned.

Example:

```go
// @description    AutoSync runs the full sync pipeline: stage and commit local changes,
//                  fetch all remotes, rebase onto the current upstream branch, then push.
//                  In case of a rebase conflict, it aborts the rebase, sends a desktop
//                  notification, and returns errRebaseFailed without pushing.
// @param           repo   "configuration for the repository to sync"
// @return          error  nil on success, or an error describing the failure"
func AutoSync(repo *RepoConfig) error {
    // ...
}
