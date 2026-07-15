package common

import (
	"errors"
	"log/slog"

	"github.com/gen2brain/beeep"
	"github.com/ztrue/tracerr"
)

// @description    Synchronizes a Git repository.
//
// AutoSync verifies the Git author, commits eligible worktree changes, fetches remotes, rebases
// onto the configured upstream, and pushes. A rebase conflict triggers a desktop alert and stops
// the pipeline before push.
//
// @param           repoConfig  "configuration for the repository to synchronize"
//
// @return          error       "nil on success, or an error from any synchronization stage or alert"
func AutoSync(repoConfig RepoConfig) error {
	var err error
	slog.Info("verifying git author", "repo", repoConfig.RepoPath)
	err = ensureGitAuthor(repoConfig)
	if err != nil {
		return tracerr.Wrap(err)
	}

	slog.Info("committing worktree changes", "repo", repoConfig.RepoPath)
	err = commit(repoConfig)
	if err != nil {
		return tracerr.Wrap(err)
	}

	slog.Info("fetching remotes", "repo", repoConfig.RepoPath)
	err = fetch(repoConfig)
	if err != nil {
		return tracerr.Wrap(err)
	}

	slog.Info("rebasing onto upstream", "repo", repoConfig.RepoPath)
	err = rebase(repoConfig)
	if err != nil {
		if errors.Is(err, errRebaseFailed) {
			repoPath := repoConfig.RepoPath
			err := beeep.Alert("Git Auto Sync - Conflict", "Could not rebase for - "+repoPath, "assets/warning.png")
			if err != nil {
				return tracerr.Wrap(err)
			}
		}
		// How should we continue?
		// - Keep sending the notification each time?
		// - Or something a bit better?
		return tracerr.Wrap(err)
	}

	slog.Info("pushing changes", "repo", repoConfig.RepoPath)
	err = push(repoConfig)
	if err != nil {
		return tracerr.Wrap(err)
	}

	// -> do a merge
	// -> push the changes

	return nil
}
