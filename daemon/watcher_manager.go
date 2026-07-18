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

// configPollInterval controls how often the daemon and each watcher check the daemon configuration
// file for changes. A removed repository stops being monitored within one interval, and an added
// repository is picked up within one interval, without restarting the daemon.
const configPollInterval = time.Minute

// localChangePollInterval controls how often each watcher polls its .git/config for [auto-sync]
// changes. It defaults to configPollInterval and is overridden in tests.
var localChangePollInterval = configPollInterval

// watcherHandle tracks a running repository watcher through its done channel.
type watcherHandle struct {
	// done is closed when the watcher goroutine exits, either because the repository was removed
	// from the daemon configuration or because the watcher returned.
	done <-chan struct{}
	// cancel stops the watcher goroutine. RestartAll and watchForLocalChange use it to restart a
	// watcher so its timing values are rebuilt from current settings.
	cancel context.CancelFunc
}

// watcherManager owns the set of running repository watchers and reconciles it against the daemon
// configuration. Removal is self-detected by each watcher (see startDaemonWatcher); the manager
// starts watchers for newly added repositories and cleans up handles for watchers that have exited.
// The recorder persists per-repository runtime status to state.json so the CLI can report it.
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
// reconcile removes handles for watchers that have exited, then starts a watcher for every
// repository in repos that is not already running. It is idempotent: an already-running
// repository is left untouched, and an exited repository is removed before it can be considered
// for a fresh start, so a repository is never monitored by two watchers at once.
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
				m.recorder.Remove(repo)
			}
		default:
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
// entries, polls the daemon configuration until the repository is removed, and runs the watcher.
// The watcher reports state transitions through onState so the manager can persist the repository's
// runtime status. The returned handle's done channel is closed when the watcher goroutine exits.
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
		go watchForRemoval(ctx, cancel, logger, repoPath)
		go watchForLocalChange(ctx, cancel, logger, repoPath)

		if err := watcher.WatchForChanges(ctx, logger, cfg, m.onState(repoPath)); err != nil {
			logger.Error("watcher exited with error", "error", err)
		}
	}()

	return &watcherHandle{done: done, cancel: cancel}
}

// @description    Builds a watcher state callback for a repository.
//
// onState returns a callback that records repoPath's runtime status through the manager's recorder.
// It returns nil when the manager has no recorder, so tests that override start with a fake run
// unchanged and the watcher skips reporting.
//
// @param           repoPath  "path to the repository whose status is recorded"
//
// @return          func(watcher.StateReport)  "state callback, or nil when no recorder is configured"
func (m *watcherManager) onState(repoPath string) func(watcher.StateReport) {
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

// @description    Cancels the watcher when its repository leaves the configuration.
//
// watchForRemoval polls the daemon configuration file's modification time and, when it changes,
// re-reads the configuration and cancels the supplied context when the repository is no longer
// listed. It returns when ctx is canceled, whether by itself or by the watcher exiting.
//
// @param           ctx       "context shared with the watcher; canceling it stops the watcher"
//
// @param           cancel    "cancellation function for ctx, called when the repository is removed"
//
// @param           logger    "repository-scoped logger"
//
// @param           repoPath  "path to the repository being watched"
func watchForRemoval(ctx context.Context, cancel context.CancelFunc, logger *slog.Logger, repoPath string) {
	ticker := time.NewTicker(configPollInterval)
	defer ticker.Stop()

	var lastMod time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mod, err := config.GlobalSettingsModTime()
			if err != nil {
				logger.Error("read daemon config mtime failed", "error", err)
				continue
			}
			if !mod.After(lastMod) {
				continue
			}
			lastMod = mod

			daemonConfig, err := config.ReadGlobalSettings()
			if err != nil {
				logger.Error("read daemon config failed", "error", err)
				continue
			}
			if !slices.Contains(daemonConfig.Repos, repoPath) {
				logger.Info("repository removed from daemon config, stopping watcher")
				cancel()
				return
			}
		}
	}
}

// @description    Restarts the watcher when its repository [auto-sync] settings change.
//
// watchForLocalChange polls <repo>/.git/config's modification time every localChangePollInterval.
// When the mtime changes, it reads the [auto-sync] settings and compares LocalFingerprint against
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
			fp := config.LocalFingerprint(local)
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

// @description    Reports whether global settings changed.
//
// settingsChanged compares the three synchronization settings between two Settings. nil and nil is
// unchanged. It ignores Repos and Envs, which the reconcile path handles separately.
//
// @param           a       "previous settings, or nil"
//
// @param           b       "current settings, or nil"
//
// @return          bool    "true if any of syncInterval, debounce, or gitexec differs"
func settingsChanged(a, b *config.Settings) bool {
	return intPtrChanged(a, b, func(s *config.Settings) *int { return s.SyncInterval }) ||
		intPtrChanged(a, b, func(s *config.Settings) *int { return s.Debounce }) ||
		strPtrChanged(a, b, func(s *config.Settings) *string { return s.GitExec })
}

// @description    Reports whether a *int field differs between two settings.
//
// intPtrChanged extracts the field via get from each settings and compares them, treating a nil
// settings as a nil field.
//
// @param           a     "previous settings, or nil"
//
// @param           b     "current settings, or nil"
//
// @param           get   "accessor returning the field to compare"
//
// @return          bool  "true when the field differs or one side is nil"
func intPtrChanged(a, b *config.Settings, get func(*config.Settings) *int) bool {
	pa, pb := getOrNil(a, get), getOrNil(b, get)
	if pa == nil && pb == nil {
		return false
	}
	if pa == nil || pb == nil {
		return true
	}
	return *pa != *pb
}

// @description    Reports whether a *string field differs between two settings.
//
// strPtrChanged extracts the field via get from each settings and compares them, treating a nil
// settings as a nil field.
//
// @param           a     "previous settings, or nil"
//
// @param           b     "current settings, or nil"
//
// @param           get   "accessor returning the field to compare"
//
// @return          bool  "true when the field differs or one side is nil"
func strPtrChanged(a, b *config.Settings, get func(*config.Settings) *string) bool {
	pa, pb := getOrNil(a, get), getOrNil(b, get)
	if pa == nil && pb == nil {
		return false
	}
	if pa == nil || pb == nil {
		return true
	}
	return *pa != *pb
}

// @description    Returns a settings field or nil when settings is nil.
//
// getOrNil guards the accessor so callers can pass a nil settings without a separate nil check.
//
// @param           s    "settings to read, or nil"
//
// @param           get  "accessor returning the field"
//
// @return          *T   "field value, or nil when s is nil"
func getOrNil[T any](s *config.Settings, get func(*config.Settings) *T) *T {
	if s == nil {
		return nil
	}
	return get(s)
}
