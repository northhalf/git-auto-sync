# Plan: Daemon config reload via config-file polling

## Goal
Let the running daemon pick up `config.json` changes **without a restart**: removed repos
self-terminate, added repos are started by the daemon. Resolves the two FIXMEs in
`daemon/main.go` (line 90 "channel to close", line 116 "signal to reload config") using a
**polling** mechanism instead of OS signals (per user direction).

## Why polling (not signals)
`daemon add`/`remove`/`env` all call `WriteDaemonConfig`, which updates the file mtime. A
lightweight mtime poll detects every change with no signal, no PID file, and no CLI wiring --
and it works cross-platform (no SIGHUP-on-Windows problem). Cost: up to one poll interval
(default 30s) of latency, and a `stat` per poll (negligible).

## Design

### 1. Watcher becomes cancellable (`internal/watcher/watch.go`) — fixes daemon/main.go:90 FIXME
- `WatchForChanges(ctx context.Context, logger *slog.Logger, cfg config.RepoConfig) error`.
- Derive `ctx, cancel := context.WithCancel(ctx)`; `defer cancel()` so the inner goroutine
  exits whenever `WatchForChanges` returns (also fixes a pre-existing goroutine leak on the
  error-return paths).
- Inner goroutine `select` adds `case <-ctx.Done(): return`. Replace `time.Sleep(cfg.FSLag)`
  with a cancellable `time.NewTimer` + select. `syncTicker` stopped via `defer`.
- Outer FS-event loop converted from a blocking receive to a `select` with `case <-ctx.Done()`
  and a cancellable send to `notifyFilteredChannel`.
- ctx is **not** threaded into `AutoSync`/`gitCommand`: an in-flight sync of a removed repo
  runs to completion, then the loop sees `ctx.Done()` and exits. This avoids killing a git
  rebase mid-flight (safer) and keeps the change localized to the watcher.
- `os.Exit(1)` on sync errors is left as-is (pre-existing, tracked by watcher.go:16 FIXME).

### 2. Config mtime helper (`internal/config/daemonconfig.go`)
- Extract `daemonConfigPath() (string, error)` shared by Read/Write/ModTime.
- Add `DaemonConfigModTime() (time.Time, error)` — returns the file's modtime, or zero + nil
  when the file does not exist.

### 3. Watcher manager + per-repo self-termination (`daemon/watcher_manager.go`, new)
- `const configPollInterval = 30 * time.Second` (tunable later).
- `watcherManager` holds `map[repoPath]*watcherHandle{ done <-chan struct{} }` + an injectable
  `start func(repo, envs) *watcherHandle` (for testing).
- `reconcile(repos, envs)`: under a mutex, first delete map entries whose `done` has fired
  (clean up self-terminated watchers), then start a watcher for any repo in `repos` not
  already in the map. Idempotent; never starts a duplicate (see race analysis below).
- `startDaemonWatcher(repo, envs)` (the real `start`): builds `RepoConfig`, appends daemon
  envs, creates a cancellable ctx, runs a poll goroutine that re-reads the daemon config on
  mtime change and `cancel()`s when the repo is no longer listed (self-termination), calls
  `watcher.WatchForChanges(ctx, ...)`, and closes `done` on return.

### 4. Daemon `run()` reconcile loop (`daemon/main.go`)
- Initial `ReadDaemonConfig` + `DaemonConfigModTime` -> `mgr.reconcile(repos, envs)`.
- Loop on a 30s ticker: on each tick, if mtime advanced, re-read config; then always
  `mgr.reconcile(...)` (cheap: cleans up exited + starts new). 
- Remove the two FIXME comments (now implemented). Drop the `sync.WaitGroup` from
  `watchForChanges` (replaced by the manager's `done` channels).

### 5. CLI `watch` (`main.go:65`)
- Pass `context.Background()` (unchanged Ctrl+C behavior; gains the leak fix).

### 6. `daemon add` no longer restarts a running daemon (`daemon_cmd.go` + `daemonservice`)
- User intent: "为什么要重启呢". With polling, a running daemon picks up the new repo within
  one poll interval, so a restart is wasteful.
- Add `Service.EnsureRunning()`: if running -> no-op; if stopped -> Start; if not installed ->
  fall back to `Enable()` (install+start).
- `daemonAdd` calls `EnsureRunning()` instead of `Enable()`. `daemonRm` is unchanged (now
  correct: a removed repo self-terminates via polling; `Disable` still fires when no repos
  remain).

## Race / duplicate analysis
- A watcher is removed from the map **only** after its `done` fires (it exited).
- A watcher exits **only** if its own poller saw the repo absent from config.
- `reconcile` cleans up exited entries **before** starting new ones.
- Therefore: while a repo is in config, its poller never cancels it -> it stays in the map ->
  `reconcile` never starts a second one. No duplicates, no orphans, robust to poll-phase skew
  between the central loop and per-repo pollers.

## Env handling (decision)
Daemon-level `Envs` are **start-time-only** for a running watcher (matches the user's
add/remove focus). Re-added or newly-added repos pick up current envs; existing unchanged
repos keep their start-time envs until restarted. Documented in code comments. (Refreshing
envs in-place would race with `AutoSync` reading `cfg.Env`; deferred.)

## Files
1. `internal/config/daemonconfig.go` — `daemonConfigPath`, `DaemonConfigModTime`.
2. `internal/watcher/watch.go` — `ctx` param + cancellable loops.
3. `main.go` — `context.Background()` for CLI `watch`.
4. `daemon/watcher_manager.go` (new) — manager, `startDaemonWatcher`, poll interval.
5. `daemon/main.go` — reconcile `run()`, drop WaitGroup/FIXMEs.
6. `daemon_cmd.go` + `internal/daemonservice/service.go` — `EnsureRunning`, `daemonAdd` uses it.
7. Tests: `internal/config/daemonconfig_test.go` (modtime); `daemon/watcher_manager_test.go`
   (reconcile add/remove/cleanup/no-duplicate via a fake `start`).

## Out of scope (follow-ups)
- `os.Exit(1)` on sync error kills the whole daemon (watcher.go:16 FIXME) — pre-existing.
- Graceful `Daemon.Stop` (today the process exit kills goroutines; unchanged).
- Direct unit test of `WatchForChanges` (AutoSync side-effects + os.Exit make it unsafe in-test;
  ctx behavior covered by code review + manager tests).

## Verification
- `go build ./...` and `make`.
- `go test ./...` (new manager + config tests).
- `golangci-lint run ./...` and `gofmt` check per CLAUDE.md.
- Manual: start daemon with repo A; `daemon add` repo B (no restart, B picked up ≤30s);
  `daemon remove` A (A's watcher stops ≤30s, B keeps running); re-add A (starts fresh).
