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
// notifications. Notifier and later sync errors are logged before terminating the process; normal
// operation loops indefinitely.
//
// @param           cfg    "repository configuration and watcher timing values"
//
// @return          error  "an error from initial sync, watcher setup, or filesystem event inspection"
func WatchForChanges(cfg config.RepoConfig) error {
	repoPath := cfg.RepoPath
	var err error

	err = syncer.AutoSync(cfg)
	if err != nil {
		return tracerr.Wrap(err)
	}

	notifyFilteredChannel := make(chan bool, 100)
	syncTicker := time.NewTicker(cfg.SyncInterval)

	// Filtered events
	go func() {
		notifier, err := NewAwakeNotifier()
		if err != nil {
			slog.Error("watcher error", "error", err)
			os.Exit(1)
		}

		err = notifier.Start(notifyFilteredChannel)
		if err != nil {
			slog.Error("watcher error", "error", err)
			os.Exit(1)
		}

		for {
			select {
			case <-notifyFilteredChannel:
				time.Sleep(cfg.FSLag)

				err := syncer.AutoSync(cfg)
				if err != nil {
					slog.Error("sync after filesystem event failed", "repo", cfg.RepoPath, "error", err)
					os.Exit(1)
				}
				continue

			case <-syncTicker.C:
				err := syncer.AutoSync(cfg)
				if err != nil {
					slog.Error("sync after ticker failed", "repo", cfg.RepoPath, "error", err)
					os.Exit(1)
				}
			}
		}
	}()

	//
	// Watch for FS events
	//
	notifyChannel := make(chan notify.EventInfo, 100)

	err = notify.Watch(filepath.Join(repoPath, "..."), notifyChannel, notify.Write, notify.Rename, notify.Remove, notify.Create)
	if err != nil {
		return tracerr.Wrap(err)
	}
	defer notify.Stop(notifyChannel)

	for {
		ei := <-notifyChannel
		ignore, err := syncer.ShouldIgnoreFile(repoPath, ei.Path())
		if err != nil {
			return tracerr.Wrap(err)
		}
		if ignore {
			continue
		}

		notifyFilteredChannel <- true
	}
}
