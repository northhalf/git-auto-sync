package syncer

import (
	"errors"
	"log/slog"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/notification"
	"github.com/ztrue/tracerr"
)

var sendAlert = notification.Alert

// @description    Synchronizes a Git repository.
//
// AutoSync verifies that the repository is in a syncable state, verifies the Git author, commits
// eligible worktree changes, and fetches the current branch's upstream branch. It then resolves
// the synchronization state between HEAD and its upstream and acts on it: equal skips rebase and
// push, local-ahead only pushes, upstream-ahead only rebases, and diverged rebases then pushes. A
// repo that has an operation in progress, a detached HEAD, or no upstream branch pauses with a
// platform alert before any mutation runs. A rebase conflict triggers a platform alert and stops
// the pipeline before push. Stage functions log their own outcomes, so AutoSync does not
// duplicate stage errors.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to synchronize"
//
// @return          error       "nil on success, or an error from any synchronization stage"
func AutoSync(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting sync")

	if err := checkRepoState(logger, repoConfig); err != nil {
		if alertErr := sendAlert("Git Auto Sync - Paused", "Sync paused for - "+repoConfig.RepoPath+": "+err.Error()); alertErr != nil && !errors.Is(alertErr, notification.ErrUnavailable) {
			// A failed platform alert is non-fatal. Log it but still report the blocking repo state so
			// daemon status shows the real reason the repository paused.
			logger.Warn("send repo state alert failed", "error", alertErr)
		}
		return tracerr.Wrap(newSyncError(repoStateStage(err), err))
	}

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
				if alertErr := sendAlert("Git Auto Sync - Conflict", "Could not rebase for - "+repoPath); alertErr != nil && !errors.Is(alertErr, notification.ErrUnavailable) {
					// A failed platform alert is non-fatal. Log it but still report the rebase
					// conflict so daemon status shows the real reason the repository paused.
					logger.Warn("send rebase conflict alert failed", "error", alertErr)
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
