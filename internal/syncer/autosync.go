package syncer

import (
	"errors"
	"log/slog"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/notification"
)

var sendAlert = notification.Alert

// @description    Sends a pause alert, tolerating headless systems.
//
// alertPaused sends a platform alert for a paused repository. A failed delivery is non-fatal: an
// unavailable notification service is expected on headless systems and stays silent, while any
// other failure is logged so daemon status still shows the real pause reason.
//
// @param           logger     "repository-scoped logger"
//
// @param           operation  "log message used when alert delivery fails"
//
// @param           title      "alert title"
//
// @param           body       "alert body text"
func alertPaused(logger *slog.Logger, operation, title, body string) {
	if err := sendAlert(title, body); err != nil && !errors.Is(err, notification.ErrUnavailable) {
		logger.Warn(operation, "error", err)
	}
}

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
		alertPaused(logger, "send repo state alert failed", "Git Auto Sync - Paused", "Sync paused for - "+repoConfig.RepoPath+": "+err.Error())
		return newSyncError(repoStateStage(err), err)
	}

	if err := ensureGitAuthor(logger, repoConfig); err != nil {
		return newSyncError(SyncStageAuthor, err)
	}

	if err := commit(logger, repoConfig); err != nil {
		return newSyncError(SyncStageCommit, err)
	}

	if err := fetch(logger, repoConfig); err != nil {
		return newSyncError(SyncStageFetch, err)
	}

	state, err := resolveUpstreamState(logger, repoConfig)
	if err != nil {
		return newSyncError(SyncStageCompare, err)
	}
	logger.Info("sync state resolved", "state", state)

	switch state {
	case upstreamStateNone, upstreamStateEqual:
		// Nothing to rebase or push: HEAD has no upstream or already matches it.
	case upstreamStateLocalAhead:
		if err := push(logger, repoConfig); err != nil {
			return newSyncError(SyncStagePush, err)
		}
	case upstreamStateUpstreamAhead, upstreamStateDiverged:
		if err := rebase(logger, repoConfig); err != nil {
			if errors.Is(err, errRebaseFailed) {
				alertPaused(logger, "send rebase conflict alert failed", "Git Auto Sync - Conflict", "Could not rebase for - "+repoConfig.RepoPath)
			}
			return newSyncError(SyncStageRebase, err)
		}
		if state == upstreamStateDiverged {
			if err := push(logger, repoConfig); err != nil {
				return newSyncError(SyncStagePush, err)
			}
		}
	}

	logger.Info("sync completed")
	return nil
}
