package syncer

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/northhalf/git-auto-sync/internal/config"
)

var (
	// errRepoBusy signals that the repository has a merge, rebase, cherry-pick, or revert in
	// progress and must not be touched until the user resolves it.
	errRepoBusy = errors.New("repository has a git operation in progress")
	// errDetachedHead signals that HEAD is not on a branch, so there is no branch to rebase or push.
	errDetachedHead = errors.New("HEAD is detached, not on a branch")
	// errNoUpstream signals that the current branch has no configured upstream tracking branch, so
	// there is nothing to rebase onto or push to.
	errNoUpstream = errors.New("current branch has no upstream tracking branch")
)

// @description    Reports the in-progress Git operation, if any.
//
// pendingGitOperation inspects the repository's .git directory for the markers Git writes while a
// multi-step operation is underway: MERGE_HEAD for an unfinished merge, rebase-merge and
// rebase-apply for an in-progress rebase, CHERRY_PICK_HEAD for a cherry-pick, and REVERT_HEAD for a
// revert. It returns a short label for the first marker found, or an empty string when the
// repository is idle. It inspects a plain repository's .git directory directly, matching isRebasing,
// and does not resolve a linked worktree whose .git is a file.
//
// @param           repoPath  "path to the repository root"
//
// @return          string    "label of the in-progress operation (merge, rebase, cherry-pick, revert), or empty when none"
func pendingGitOperation(repoPath string) string {
	gitDir := filepath.Join(repoPath, ".git")
	markers := []struct {
		name  string
		label string
	}{
		{"MERGE_HEAD", "merge"},
		{"rebase-merge", "rebase"},
		{"rebase-apply", "rebase"},
		{"CHERRY_PICK_HEAD", "cherry-pick"},
		{"REVERT_HEAD", "revert"},
	}
	for _, m := range markers {
		exists, err := pathExists(filepath.Join(gitDir, m.name))
		if err != nil {
			// A stat error other than NotExist is unexpected; treat the marker as absent so the
			// caller proceeds and the later go-git open surfaces any real repository failure.
			continue
		}
		if exists {
			return m.label
		}
	}
	return ""
}

// @description    Validates that the repository is in a syncable state.
//
// checkRepoState is the first stage of AutoSync. It rejects repositories that AutoSync must not
// touch: one with a Git operation in progress (merge, rebase, cherry-pick, or revert), a detached
// HEAD, or a current branch without a configured upstream tracking branch. Returning before commit,
// fetch, and rebase keeps a repository in any of these states paused until the user resolves it and
// restarts the daemon.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to inspect"
//
// @return          error       "nil when the repository is syncable, or a sentinel error describing the blocking state"
func checkRepoState(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting repo state check")

	if op := pendingGitOperation(repoConfig.RepoPath); op != "" {
		logger.Error("repo state check failed", "reason", "operation in progress", "operation", op)
		return fmt.Errorf("%w: %s", errRepoBusy, op)
	}

	bi, err := readBranchInfo(repoConfig.RepoPath)
	if err != nil {
		logger.Error("repo state check failed", "operation", "read branch information", "error", err)
		return err
	}

	// A detached HEAD resolves to no branch name; check it before the upstream so the more
	// specific reason is reported.
	if bi.CurrentBranch == "" {
		logger.Error("repo state check failed", "reason", "detached HEAD")
		return errDetachedHead
	}

	if !bi.hasUpstream() {
		logger.Error("repo state check failed", "reason", "no upstream", "branch", bi.CurrentBranch)
		return errNoUpstream
	}

	logger.Info("repo state verified", "branch", bi.CurrentBranch, "remote", bi.UpstreamRemote)
	return nil
}

// @description    Maps a checkRepoState error to its daemon-state stage label.
//
// repoStateStage returns the specific stage label for the sentinel error produced by checkRepoState
// so the watcher records why the repository paused: repo-busy, detached-head, or no-upstream. It
// falls back to the generic repo-state stage for any other error, such as a repository that could
// not be opened, so an unexpected failure is still attributed to the repo-state stage.
//
// @param           err    "error returned from checkRepoState"
//
// @return          string "stage label: repo-busy, detached-head, no-upstream, or repo-state"
func repoStateStage(err error) string {
	switch {
	case errors.Is(err, errRepoBusy):
		return SyncStageRepoBusy
	case errors.Is(err, errDetachedHead):
		return SyncStageDetachedHead
	case errors.Is(err, errNoUpstream):
		return SyncStageNoUpstream
	default:
		return SyncStageRepoState
	}
}
