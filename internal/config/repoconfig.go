package config

import (
	"os"
	"strconv"
	"time"

	"github.com/ztrue/tracerr"
	"gopkg.in/src-d/go-git.v4"
)

type RepoConfig struct {
	RepoPath     string
	SyncInterval time.Duration
	FSLag        time.Duration
	GitExec      string
	Env          []string
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
