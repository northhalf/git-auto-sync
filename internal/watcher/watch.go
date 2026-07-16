package watcher

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/rjeczalik/notify"
	"github.com/ztrue/tracerr"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/syncer"
)

var watcherRetryDelays = []time.Duration{
	2 * time.Minute,
	4 * time.Minute,
	8 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	60 * time.Minute,
}

type AwakeNotifier interface {
	Start(context.Context, chan<- bool) error
}

// @description    Synchronizes and watches a repository.
//
// WatchForChanges starts filesystem and wake notifications before the initial synchronization, then
// runs a repository-local state machine. Fetch and push errors use capped retry backoff; errors from
// other synchronization stages pause the repository until the watcher context is canceled. A
// failure in one repository never terminates the daemon process or another repository watcher.
//
// @param           ctx     "context whose cancellation stops the watcher and releases its resources"
//
// @param           logger  "repository-scoped logger"
//
// @param           cfg     "repository configuration and watcher timing values"
//
// @return          error   "an error from watcher setup or filesystem event inspection"
func WatchForChanges(ctx context.Context, logger *slog.Logger, cfg config.RepoConfig) error {
	logger.Debug("starting watcher")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	notifyChannel := make(chan notify.EventInfo, 100)
	if err := notify.Watch(filepath.Join(cfg.RepoPath, "..."), notifyChannel, notify.Write, notify.Rename, notify.Remove, notify.Create); err != nil {
		logger.Error("watcher failed", "operation", "watch filesystem", "error", err)
		return tracerr.Wrap(err)
	}
	defer notify.Stop(notifyChannel)

	awakeChannel := make(chan bool, 100)
	notifier, err := NewAwakeNotifier()
	if err != nil {
		logger.Error("watcher failed", "operation", "create awake notifier", "error", err)
		return tracerr.Wrap(err)
	}
	if err := notifier.Start(ctx, awakeChannel); err != nil {
		logger.Error("watcher failed", "operation", "start awake notifier", "error", err)
		return tracerr.Wrap(err)
	}

	syncTicker := time.NewTicker(cfg.SyncInterval)
	defer syncTicker.Stop()

	deps := watchDependencies{
		autoSync: func() error {
			return syncer.AutoSync(logger, cfg)
		},
		shouldIgnore: func(path string) (bool, error) {
			return syncer.ShouldIgnoreFile(cfg.RepoPath, path)
		},
		isRemoteSyncError: syncer.IsRemoteSyncError,
		syncErrorStage:    syncer.SyncErrorStage,
		retryDelays:       watcherRetryDelays,
	}

	logger.Info("watcher started")
	if err := runWatchLoop(ctx, logger, cfg, notifyChannel, awakeChannel, syncTicker.C, deps); err != nil {
		return tracerr.Wrap(err)
	}
	return nil
}
