package syncer

import (
	"errors"
	"log/slog"

	"github.com/gen2brain/beeep"
	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
)

// @description    Synchronizes a Git repository.
//
// AutoSync verifies the Git author, commits eligible worktree changes, fetches remotes, rebases
// onto the configured upstream, and pushes. A rebase conflict triggers a desktop alert and stops
// the pipeline before push. Stage functions log their own outcomes, so AutoSync does not duplicate
// stage errors.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to synchronize"
//
// @return          error       "nil on success, or an error from any synchronization stage or alert"
func AutoSync(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting sync")

	if err := ensureGitAuthor(logger, repoConfig); err != nil {
		return tracerr.Wrap(err)
	}

	if err := commit(logger, repoConfig); err != nil {
		return tracerr.Wrap(err)
	}

	if err := fetch(logger, repoConfig); err != nil {
		return tracerr.Wrap(err)
	}

	if err := rebase(logger, repoConfig); err != nil {
		if errors.Is(err, errRebaseFailed) {
			repoPath := repoConfig.RepoPath
			alertErr := beeep.Alert("Git Auto Sync - Conflict", "Could not rebase for - "+repoPath, "assets/warning.png")
			if alertErr != nil {
				logger.Error("send rebase conflict alert failed", "error", alertErr)
				return tracerr.Wrap(alertErr)
			}
		}
		return tracerr.Wrap(err)
	}

	if err := push(logger, repoConfig); err != nil {
		return tracerr.Wrap(err)
	}

	logger.Info("sync completed")
	return nil
}
