package syncer

import (
	"errors"
	"log/slog"

	"github.com/gen2brain/beeep"
	"github.com/northhalf/git-auto-sync/assets"
	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
)

// @description    Synchronizes a Git repository.
//
// AutoSync verifies the Git author, commits eligible worktree changes, and fetches remotes. It then
// resolves the synchronization state between HEAD and its upstream and acts on it: equal or no
// upstream skips rebase and push, local-ahead only pushes, upstream-ahead only rebases, and
// diverged rebases then pushes. A rebase conflict triggers a desktop alert and stops the pipeline
// before push. Stage functions log their own outcomes, so AutoSync does not duplicate stage errors.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to synchronize"
//
// @return          error       "nil on success, or an error from any synchronization stage or alert"
func AutoSync(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting sync")

	if err := ensureGitAuthor(logger, repoConfig); err != nil {
		return tracerr.Wrap(newSyncError(syncStageAuthor, err))
	}

	if err := commit(logger, repoConfig); err != nil {
		return tracerr.Wrap(newSyncError(syncStageCommit, err))
	}

	if err := fetch(logger, repoConfig); err != nil {
		return tracerr.Wrap(newSyncError(syncStageFetch, err))
	}

	state, err := resolveUpstreamState(logger, repoConfig)
	if err != nil {
		return tracerr.Wrap(newSyncError(syncStageCompare, err))
	}
	logger.Info("sync state resolved", "state", state)

	switch state {
	case upstreamStateNone, upstreamStateEqual:
		// Nothing to rebase or push: HEAD has no upstream or already matches it.
	case upstreamStateLocalAhead:
		if err := push(logger, repoConfig); err != nil {
			return tracerr.Wrap(newSyncError(syncStagePush, err))
		}
	case upstreamStateUpstreamAhead, upstreamStateDiverged:
		if err := rebase(logger, repoConfig); err != nil {
			if errors.Is(err, errRebaseFailed) {
				repoPath := repoConfig.RepoPath
				alertErr := beeep.Alert("Git Auto Sync - Conflict", "Could not rebase for - "+repoPath, assets.WarningPNG)
				if alertErr != nil {
					logger.Error("send rebase conflict alert failed", "error", alertErr)
					return tracerr.Wrap(newSyncError(syncStageAlert, alertErr))
				}
			}
			return tracerr.Wrap(newSyncError(syncStageRebase, err))
		}
		if state == upstreamStateDiverged {
			if err := push(logger, repoConfig); err != nil {
				return tracerr.Wrap(newSyncError(syncStagePush, err))
			}
		}
	}

	logger.Info("sync completed")
	return nil
}
