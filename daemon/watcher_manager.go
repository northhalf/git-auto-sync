package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/daemonstate"
	"github.com/northhalf/git-auto-sync/internal/logging"
	"github.com/northhalf/git-auto-sync/internal/watcher"
)

// configPollInterval controls how often the daemon checks the daemon configuration file for
// changes. A removed repository's watcher is canceled within one interval, and an added repository
// is picked up within one interval, without restarting the daemon.
const configPollInterval = time.Minute

// localChangePollInterval controls how often each watcher polls its .git/config for [auto-sync]
// changes. It defaults to configPollInterval and is overridden in tests.
var localChangePollInterval = configPollInterval

// watcherHandle tracks a running repository watcher through its done channel.
type watcherHandle struct {
	// done is closed when the watcher goroutine exits, either because its context was canceled or
	// because the watcher returned.
	done <-chan struct{}
	// cancel stops the watcher goroutine. reconcile uses it when the repository leaves the daemon
	// configuration, and RestartAll and watchForLocalChange use it to restart a watcher so its
	// timing values are rebuilt from current settings.
	cancel context.CancelFunc
}

// watcherManager owns the set of running repository watchers and reconciles it against the daemon
// configuration: it cancels watchers whose repository left the configuration, starts watchers for
// newly added repositories, and cleans up handles for watchers that have exited. The recorder
// persists per-repository runtime status to state.json so the CLI can report it.
type watcherManager struct {
	mu       sync.Mutex
	watchers map[string]*watcherHandle
	start    func(repoPath string, envs []string) *watcherHandle
	recorder *daemonstate.Recorder
}

// @description    Creates the daemon watcher manager.
//
// newWatcherManager returns a manager that starts repository watchers with startDaemonWatcher and
// records their runtime status through a new daemon state recorder. The start field is overridable
// in tests.
//
// @return          *watcherManager  "manager backed by startDaemonWatcher and a state recorder"
func newWatcherManager() *watcherManager {
	m := &watcherManager{
		watchers: make(map[string]*watcherHandle),
		recorder: daemonstate.NewRecorder(),
	}
	m.start = m.startDaemonWatcher
	return m
}

// @description    Reconciles running watchers against the configuration.
//
// reconcile removes handles for watchers that have exited, cancels every watcher whose repository
// is no longer listed, then starts a watcher for every repository in repos that is not already
// running. A canceled watcher exits asynchronously and its handle is cleaned up on a later pass.
// An unexpected exit forgets only the active heartbeat so a replacement can recover the persisted
// LastSyncedAt; an exit after configuration removal deletes the repository state. An
// already-running repository is left untouched, and an exited repository is removed before restart,
// so two watchers never monitor one repository.
//
// @param           repos  "repository paths that should be monitored"
//
// @param           envs   "daemon-level environment entries applied to newly started watchers"
func (m *watcherManager) reconcile(repos []string, envs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for repo, handle := range m.watchers {
		select {
		case <-handle.done:
			delete(m.watchers, repo)
			if m.recorder != nil {
				if slices.Contains(repos, repo) {
					m.recorder.Forget(repo)
				} else {
					m.recorder.Remove(repo)
				}
			}
			continue
		default:
		}
		if !slices.Contains(repos, repo) {
			handle.cancel()
		}
	}

	for _, repo := range repos {
		if _, exists := m.watchers[repo]; exists {
			continue
		}
		m.watchers[repo] = m.start(repo, envs)
	}
}

// @description    Starts one repository watcher.
//
// startDaemonWatcher builds the repository configuration, applies the daemon's environment
// entries, and runs the watcher until the manager cancels its context. The watcher reports state
// transitions through stateReporter so the manager can persist the repository's runtime status. The
// returned handle's done channel is closed when the watcher goroutine exits.
//
// @param           repoPath  "path to the repository to watch"
//
// @param           envs      "daemon-level environment entries applied to the repository configuration"
//
// @return          *watcherHandle  "handle whose done channel closes on watcher exit"
func (m *watcherManager) startDaemonWatcher(repoPath string, envs []string) *watcherHandle {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer cancel()

		logger := logging.WithRepo(repoPath)
		cfg, err := config.NewRepoConfig(repoPath)
		if err != nil {
			logger.Error("build repo config failed", "error", err)
			return
		}
		logger.Info("monitoring repo")
		cfg.Env = append(cfg.Env, envs...)

		// Daemon-level environment entries are captured at start time. A change to the daemon
		// environment does not refresh a running watcher; remove and re-add the repository, or
		// restart the daemon, to apply new environment entries to an existing repository.
		go watchForLocalChange(ctx, cancel, logger, repoPath)

		if err := watcher.WatchForChanges(ctx, logger, cfg, m.stateReporter(repoPath)); err != nil {
			logger.Error("watcher exited with error", "error", err)
		}
	}()

	return &watcherHandle{done: done, cancel: cancel}
}

// @description    Builds a watcher state callback for a repository.
//
// stateReporter returns a callback that records repoPath's runtime status through the manager's recorder.
// It returns nil when the manager has no recorder, so tests that override start with a fake run
// unchanged and the watcher skips reporting.
//
// @param           repoPath  "path to the repository whose status is recorded"
//
// @return          func(watcher.StateReport)  "state callback, or nil when no recorder is configured"
func (m *watcherManager) stateReporter(repoPath string) func(watcher.StateReport) {
	recorder := m.recorder
	if recorder == nil {
		return nil
	}
	return func(r watcher.StateReport) {
		status := daemonstate.StatusRunning
		stage := ""
		if r.Paused {
			status = daemonstate.StatusPaused
			stage = r.Stage
		}
		recorder.Set(repoPath, status, stage, r.LastSyncedAt)
	}
}

// @description    Refreshes the heartbeat for every running watcher.
//
// Heartbeat bumps the persisted UpdatedAt timestamp for every tracked repository so the CLI can
// distinguish a live daemon from one that has stopped. It is a no-op when the manager has no
// recorder.
func (m *watcherManager) Heartbeat() {
	if m.recorder != nil {
		m.recorder.Heartbeat()
	}
}

// @description    Restarts the watcher when its repository [auto-sync] settings change.
//
// watchForLocalChange polls <repo>/.git/config's modification time every localChangePollInterval.
// When the mtime changes, it reads the [auto-sync] settings and compares SyncSettingsFingerprint against
// the last fingerprint. Only a fingerprint change cancels the watcher context, so routine fetch,
// rebase, and commit activity that rewrites other .git/config sections does not trigger a restart.
// The first observation establishes the baseline fingerprint without canceling, so a repository
// with pre-existing [auto-sync] settings does not restart its watcher on startup.
//
// @param           ctx       "context shared with the watcher; canceling it stops the watcher"
//
// @param           cancel    "cancellation function called when [auto-sync] changes"
//
// @param           logger    "repository-scoped logger"
//
// @param           repoPath  "path to the repository being watched"
func watchForLocalChange(ctx context.Context, cancel context.CancelFunc, logger *slog.Logger, repoPath string) {
	ticker := time.NewTicker(localChangePollInterval)
	defer ticker.Stop()

	gitConfigPath := filepath.Join(repoPath, ".git", "config")
	var lastMod time.Time
	var lastFingerprint string
	initialized := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			info, err := os.Stat(gitConfigPath)
			if err != nil {
				if !os.IsNotExist(err) {
					logger.Error("stat git config failed", "error", err)
				}
				continue
			}
			mod := info.ModTime()
			if !mod.After(lastMod) {
				continue
			}
			lastMod = mod

			local, err := config.ReadLocalSettings(repoPath)
			if err != nil {
				logger.Error("read local settings failed", "error", err)
				continue
			}
			fp := config.SyncSettingsFingerprint(local)
			if !initialized {
				// First observation: establish the baseline fingerprint without canceling.
				lastFingerprint = fp
				initialized = true
				continue
			}
			if fp == lastFingerprint {
				continue
			}
			logger.Info("local auto-sync settings changed, restarting watcher")
			cancel()
			return
		}
	}
}

// @description    Cancels and clears every running watcher.
//
// RestartAll cancels each watcher's context and empties the handle map so the next reconcile pass
// restarts every still-listed repository. It is used when global settings change, because global
// settings affect every repository that has not set a local override.
func (m *watcherManager) RestartAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, handle := range m.watchers {
		handle.cancel()
	}
	m.watchers = make(map[string]*watcherHandle)
}
