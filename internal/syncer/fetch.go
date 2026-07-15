package syncer

import (
	"log/slog"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
	git "gopkg.in/src-d/go-git.v4"
)

// @description    Fetches every configured remote.
//
// fetch runs Git fetch for every remote configured in the repository, stopping at the first
// repository or command error. Remote names may be logged, but remote URLs are never logged.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to fetch"
//
// @return          error       "nil when all remotes are fetched, or the first encountered error"
func fetch(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting fetch")

	r, err := git.PlainOpenWithOptions(repoConfig.RepoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		logger.Error("fetch failed", "operation", "open repository", "error", err)
		return tracerr.Wrap(err)
	}

	remotes, err := r.Remotes()
	if err != nil {
		logger.Error("fetch failed", "operation", "list remotes", "error", err)
		return tracerr.Wrap(err)
	}

	if len(remotes) == 0 {
		logger.Info("fetch skipped", "reason", "no remotes")
		return nil
	}

	for _, remote := range remotes {
		remoteName := remote.Config().Name
		if _, err := gitCommand(logger, repoConfig, []string{"fetch", remoteName}); err != nil {
			logger.Error("fetch failed", "remote", remoteName, "error", err)
			return tracerr.Wrap(err)
		}
	}

	logger.Info("fetch completed", "remotes", len(remotes))
	return nil
}
