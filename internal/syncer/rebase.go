package syncer

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
)

var errRebaseFailed = errors.New("git rebase failed")

// @description    Rebases onto the configured upstream.
//
// rebase rebases the current branch onto its configured upstream. It skips repositories without a
// configured upstream. If Git leaves a rebase in progress after an exit-code-one failure, it aborts
// the rebase and returns errRebaseFailed.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to rebase"
//
// @return          error       "nil on success or no upstream, errRebaseFailed after a conflict, or another error"
func rebase(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting rebase")

	repoPath := repoConfig.RepoPath
	bi, err := fetchBranchInfo(repoPath)
	if err != nil {
		logger.Error("rebase failed", "operation", "read branch information", "error", err)
		return tracerr.Wrap(err)
	}

	if bi.UpstreamRemote == "" || bi.UpstreamBranch == "" {
		logger.Info("rebase skipped", "reason", "no upstream")
		return nil
	}

	_, rebaseErr := gitCommand(logger, repoConfig, []string{"rebase", bi.UpstreamRemote + "/" + bi.UpstreamBranch})
	if rebaseErr != nil {
		rebaseInProgress, err := isRebasing(repoPath)
		if err != nil {
			logger.Error("rebase failed", "operation", "inspect rebase state", "error", err)
			return tracerr.Wrap(err)
		}

		var exerr *exec.ExitError
		if errors.As(rebaseErr, &exerr) && exerr.ExitCode() == 1 && rebaseInProgress {
			if _, err := gitCommand(logger, repoConfig, []string{"rebase", "--abort"}); err != nil {
				logger.Error("rebase failed", "operation", "abort rebase", "error", err)
				return tracerr.Wrap(err)
			}
			logger.Error("rebase failed", "error", errRebaseFailed)
			return errRebaseFailed
		}
		logger.Error("rebase failed", "remote", bi.UpstreamRemote, "branch", bi.UpstreamBranch, "error", rebaseErr)
		return tracerr.Wrap(rebaseErr)
	}

	logger.Info("rebase completed", "remote", bi.UpstreamRemote, "branch", bi.UpstreamBranch)
	return nil
}

// @description    Checks whether a filesystem path exists.
//
// exists reports whether a filesystem path exists and distinguishes absence from other stat
// errors.
//
// @param           name   "filesystem path to inspect"
//
// @return          bool   "true when the path exists"
//
// @return          error  "nil for existing or absent paths, or the filesystem error"
func pathExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// @description    isRebasing checks for Git's rebase-apply and rebase-merge state directories.
//
// @param           repoPath  "path to the repository root"
//
// @return          bool      "true when either rebase state directory exists"
//
// @return          error     "nil on success, or an error inspecting a state directory"
func isRebasing(repoPath string) (bool, error) {
	ra, err := pathExists(path.Join(repoPath, ".git", "rebase-apply"))
	if err != nil {
		return false, tracerr.Wrap(err)
	}

	rm, err := pathExists(path.Join(repoPath, ".git", "rebase-merge"))
	if err != nil {
		return false, tracerr.Wrap(err)
	}

	return ra || rm, nil
}
