package common

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rjeczalik/notify"
	"github.com/ztrue/tracerr"
	"gopkg.in/src-d/go-git.v4"
)

// FIXME: Replace the logger with returning an error and retrying after 'x' minutes

type RepoConfig struct {
	RepoPath     string
	SyncInterval time.Duration
	FSLag        time.Duration
	GitExec      string
	Env          []string
}

type AwakeNotifier interface {
	Start(chan bool) error
}

// @description    Reads repository synchronization settings.
//
// NewRepoConfig reads repository-local auto-sync settings, applies default timing values, and
// checks that any configured Git executable path exists.
//
// @param           repoPath    "path to the repository root"
//
// @return          RepoConfig  "resolved repository configuration"
//
// @return          error       "nil on success, or an error reading Git configuration or the executable"
func NewRepoConfig(repoPath string) (RepoConfig, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return RepoConfig{}, tracerr.Wrap(err)
	}

	config, err := repo.Config()
	if err != nil {
		return RepoConfig{}, tracerr.Wrap(err)
	}

	autoSyncSection := config.Raw.Section("auto-sync")

	syncInterval := 10 * time.Minute
	if autoSyncSection.Option("syncInterval") != "" {
		secondsStr := autoSyncSection.Option("syncInterval")
		seconds, err := strconv.Atoi(secondsStr)
		if err != nil {
			return RepoConfig{}, tracerr.Wrap(err)
		}

		syncInterval = time.Duration(seconds) * time.Second
	}

	gitExec := ""
	if autoSyncSection.Option("exec") != "" {
		gitExec = autoSyncSection.Option("exec")

		_, err := os.Stat(gitExec)
		if err != nil {
			return RepoConfig{}, tracerr.Wrap(err)
		}
	}

	return RepoConfig{
		RepoPath:     repoPath,
		SyncInterval: syncInterval,
		FSLag:        1 * time.Second,
		GitExec:      gitExec,
	}, nil
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
func WatchForChanges(cfg RepoConfig) error {
	repoPath := cfg.RepoPath
	var err error

	err = AutoSync(cfg)
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

				err := AutoSync(cfg)
				if err != nil {
					slog.Error("sync after filesystem event failed", "repo", cfg.RepoPath, "error", err)
					os.Exit(1)
				}
				continue

			case <-syncTicker.C:
				err := AutoSync(cfg)
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
		ignore, err := ShouldIgnoreFile(repoPath, ei.Path())
		if err != nil {
			return tracerr.Wrap(err)
		}
		if ignore {
			continue
		}

		notifyFilteredChannel <- true
	}
}
