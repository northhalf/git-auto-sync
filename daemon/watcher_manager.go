package main

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/logging"
	"github.com/northhalf/git-auto-sync/internal/watcher"
)

// configPollInterval controls how often the daemon and each watcher check the daemon configuration
// file for changes. A removed repository stops being monitored within one interval, and an added
// repository is picked up within one interval, without restarting the daemon.
const configPollInterval = 30 * time.Second

// watcherHandle tracks a running repository watcher through its done channel.
type watcherHandle struct {
	// done is closed when the watcher goroutine exits, either because the repository was removed
	// from the daemon configuration or because the watcher returned.
	done <-chan struct{}
}

// watcherManager owns the set of running repository watchers and reconciles it against the daemon
// configuration. Removal is self-detected by each watcher (see startDaemonWatcher); the manager
// starts watchers for newly added repositories and cleans up handles for watchers that have exited.
type watcherManager struct {
	mu       sync.Mutex
	watchers map[string]*watcherHandle
	start    func(repoPath string, envs []string) *watcherHandle
}

// @description    Creates the daemon watcher manager.
//
// newWatcherManager returns a manager that starts repository watchers with startDaemonWatcher.
// The start field is overridable in tests.
//
// @return          *watcherManager  "manager backed by startDaemonWatcher"
func newWatcherManager() *watcherManager {
	return &watcherManager{
		watchers: make(map[string]*watcherHandle),
		start:    startDaemonWatcher,
	}
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
// The returned handle's done channel is closed when the watcher goroutine exits.
//
// @param           repoPath  "path to the repository to watch"
//
// @param           envs      "daemon-level environment entries applied to the repository configuration"
//
// @return          *watcherHandle  "handle whose done channel closes on watcher exit"
func startDaemonWatcher(repoPath string, envs []string) *watcherHandle {
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

		if err := watcher.WatchForChanges(ctx, logger, cfg); err != nil {
			logger.Error("watcher exited with error", "error", err)
		}
	}()

	return &watcherHandle{done: done}
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
			mod, err := config.DaemonConfigModTime()
			if err != nil {
				logger.Error("read daemon config mtime failed", "error", err)
				continue
			}
			if !mod.After(lastMod) {
				continue
			}
			lastMod = mod

			daemonConfig, err := config.ReadDaemonConfig()
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
