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
go test ./internal/syncer

# Run one test by exact name
go test ./internal/syncer -run '^Test_NewFile$'

# Disable the test cache while iterating
go test -count=1 ./internal/syncer -run '^Test_RebaseBothCommitsConflict$'

# Run the CI linter
golangci-lint run ./...

# Check formatting without modifying files
test -z "$(gofmt -l -- *.go daemon/*.go internal/config/*.go internal/daemonservice/*.go internal/logging/*.go internal/syncer/*.go internal/watcher/*.go)"

# Apply formatting
gofmt -w -- *.go daemon/*.go internal/config/*.go internal/daemonservice/*.go internal/logging/*.go internal/syncer/*.go internal/watcher/*.go
```

The integration-style tests under `internal/syncer` copy repositories from `internal/syncer/testdata` into temporary directories. Fixtures store repository metadata as `.gitted`; test helpers rename it to `.git` after copying. Fetch and rebase fixtures also rewrite `$TESTDATA$` remote paths. Preserve that layout when adding fixture cases.

## Architecture

The project produces two executables:

- The root `main` package builds `git-auto-sync`, the user-facing CLI. `main.go` defines `watch`, `sync`, `check`, and `daemon` commands. `daemon_cmd.go` implements daemon configuration and service-management subcommands, while `repository.go` contains shared CLI repository validation.
- `daemon/main.go` builds `git-auto-sync-daemon`, the service process. It reads the configured repositories, starts one watcher goroutine per repository, and runs through `kardianos/service`. The service setup expects this binary beside `git-auto-sync`.

Shared behavior is organized under `internal` by responsibility:

1. `internal/config` stores daemon configuration and reads per-repository settings from the Git config section `[auto-sync]`. `syncInterval` controls periodic syncs in seconds, and `exec` selects a Git executable. The CLI can append explicit `--env KEY=VALUE` entries for a manual sync.
2. `internal/syncer` implements `AutoSync`: verify `user.email` and `user.name`, commit eligible worktree changes, fetch every remote, rebase onto the current branch's configured upstream, then push to that upstream.
3. The syncer uses `go-git` for repository discovery, status, staging, branch configuration, and ignore matching. Mutating and network operations run through the package-private `gitCommand`, which controls the working directory and environment.
4. A rebase conflict triggers `git rebase --abort`, returns `errRebaseFailed`, and causes `AutoSync` to send a desktop notification. The sync stops before push.
5. `internal/watcher` implements `WatchForChanges`, combining recursive filesystem notifications, a periodic ticker, and platform-specific wake notifications. It uses `syncer.ShouldIgnoreFile` before requesting another sync.
6. `internal/logging` configures CLI and daemon logs, while `internal/daemonservice` wraps user-service installation and lifecycle operations.

Daemon state is separate from repository-local settings. `internal/config/daemonconfig.go` stores watched repository paths and daemon environment entries in the platform-local `git-auto-sync/config.json`.

`ShouldIgnoreFile` is shared by the watcher and commit path. It excludes editor swap/backup files, `.git` contents, empty files, and files matched by Git ignore/exclude rules. Keep event filtering and commit filtering aligned when changing ignore behavior.

Platform wake behavior relies on Go filename suffixes: Darwin uses `mac-sleep-notifier`; Linux and Windows currently provide no-op implementations. Release builds enable CGO for Darwin because the filesystem notification dependency requires it, while Linux and Windows builds disable CGO.

## Code style

All functions — especially exported ones — **must** include structured doc comments using the following tag-based style:

- `@description` – place a brief summary on the tag line. When more context is needed, add a blank comment line and a separate detail paragraph that starts with the function name.
- `@param` – add one entry for each parameter: its name and a short explanation (for example, `"path to repository root"`).
- `@return` – add one entry for each return value: its name, when available, and what it represents or when it is returned.
- Insert a blank `//` comment line between every two `@...` tag lines, including consecutive `@param` and `@return` entries. IDE documentation renderers otherwise join the tags into one paragraph.
- Keep blank `//` comment lines around the detail paragraph as well.
- Write detail paragraphs as regular `// text` comments. Do not align continuation lines under `@description`; `gofmt` converts those lines into indented comment text and visually separates them from the tag line.

Example:

```go
// @description    Synchronizes a Git repository.
//
// AutoSync verifies the Git author, commits eligible changes, fetches all remotes, rebases onto
// the configured upstream branch, and pushes. A rebase conflict aborts the rebase, sends a desktop
// notification, and stops the pipeline before push.
//
// @param           repoConfig  "configuration for the repository to synchronize"
//
// @return          error       "nil on success, or an error from a synchronization stage"
func AutoSync(repoConfig RepoConfig) error {
    // ...
}
```
