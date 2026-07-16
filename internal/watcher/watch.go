package watcher

import (
	"context"
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
// duplicated here. Normal operation loops until ctx is canceled, at which point the filesystem
// watcher and tickers are stopped and the function returns nil.
//
// @param           ctx     "context whose cancellation stops the watcher and releases its resources"
//
// @param           logger  "repository-scoped logger"
//
// @param           cfg     "repository configuration and watcher timing values"
//
// @return          error   "an error from initial sync, watcher setup, or filesystem event inspection"
func WatchForChanges(ctx context.Context, logger *slog.Logger, cfg config.RepoConfig) error {
	logger.Debug("starting watcher")

	repoPath := cfg.RepoPath
	if err := syncer.AutoSync(logger, cfg); err != nil {
		return tracerr.Wrap(err)
	}

	// Derive a cancellable context so the inner goroutine stops when WatchForChanges returns,
	// including the error-return paths that previously leaked it.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	notifyFilteredChannel := make(chan bool, 100)
	syncTicker := time.NewTicker(cfg.SyncInterval)
	defer syncTicker.Stop()

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
			case <-ctx.Done():
				return

			case <-notifyFilteredChannel:
				// Wait for filesystem writes to settle, but remain cancellable while waiting.
				lag := time.NewTimer(cfg.FSLag)
				select {
				case <-ctx.Done():
					lag.Stop()
					return
				case <-lag.C:
				}

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
		select {
		case <-ctx.Done():
			return nil

		case ei := <-notifyChannel:
			ignore, err := syncer.ShouldIgnoreFile(repoPath, ei.Path())
			if err != nil {
				logger.Error("watcher failed", "operation", "inspect filesystem event", "path", ei.Path(), "error", err)
				return tracerr.Wrap(err)
			}
			if ignore {
				logger.Debug("filesystem event skipped", "path", ei.Path())
				continue
			}

			select {
			case notifyFilteredChannel <- true:
			case <-ctx.Done():
				return nil
			}
		}
	}
}
