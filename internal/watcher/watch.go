package watcher

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/rjeczalik/notify"
	"github.com/ztrue/tracerr"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/syncer"
)

// FIXME: Replace the logger with returning an error and retrying after 'x' minutes

type AwakeNotifier interface {
	Start(chan bool) error
}

// @description    Synchronizes and watches a repository.
//
// WatchForChanges performs an initial sync, then watches recursive filesystem events while a
// goroutine requests further syncs from eligible events, a periodic ticker, and platform wake
// notifications. Sync errors are already logged by the failing synchronization stage and are not
// duplicated here. Normal operation loops indefinitely.
//
// @param           logger  "repository-scoped logger"
//
// @param           cfg     "repository configuration and watcher timing values"
//
// @return          error   "an error from initial sync, watcher setup, or filesystem event inspection"
func WatchForChanges(logger *slog.Logger, cfg config.RepoConfig) error {
	logger.Debug("starting watcher")

	repoPath := cfg.RepoPath
	if err := syncer.AutoSync(logger, cfg); err != nil {
		return tracerr.Wrap(err)
	}

	notifyFilteredChannel := make(chan bool, 100)
	syncTicker := time.NewTicker(cfg.SyncInterval)

	// Filtered events
	go func() {
		notifier, err := NewAwakeNotifier()
		if err != nil {
			logger.Error("watcher failed", "operation", "create awake notifier", "error", err)
			os.Exit(1)
		}

		err = notifier.Start(notifyFilteredChannel)
		if err != nil {
			logger.Error("watcher failed", "operation", "start awake notifier", "error", err)
			os.Exit(1)
		}

		for {
			select {
			case <-notifyFilteredChannel:
				time.Sleep(cfg.FSLag)

				if err := syncer.AutoSync(logger, cfg); err != nil {
					os.Exit(1)
				}
				continue

			case <-syncTicker.C:
				if err := syncer.AutoSync(logger, cfg); err != nil {
					os.Exit(1)
				}
			}
		}
	}()

	// Watch for FS events.
	notifyChannel := make(chan notify.EventInfo, 100)

	if err := notify.Watch(filepath.Join(repoPath, "..."), notifyChannel, notify.Write, notify.Rename, notify.Remove, notify.Create); err != nil {
		logger.Error("watcher failed", "operation", "watch filesystem", "error", err)
		return tracerr.Wrap(err)
	}
	defer notify.Stop(notifyChannel)

	logger.Info("watcher started")
	for {
		ei := <-notifyChannel
		ignore, err := syncer.ShouldIgnoreFile(repoPath, ei.Path())
		if err != nil {
			logger.Error("watcher failed", "operation", "inspect filesystem event", "path", ei.Path(), "error", err)
			return tracerr.Wrap(err)
		}
		if ignore {
			logger.Debug("filesystem event skipped", "path", ei.Path())
			continue
		}

		notifyFilteredChannel <- true
	}
}
