package common

import (
	"errors"
	"os"
	"os/exec"
	"path"

	"github.com/ztrue/tracerr"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

var errRebaseFailed = errors.New("git rebase failed")

// @description    Rebases onto the configured upstream.
//
// rebase rebases the current branch onto its configured upstream. If Git leaves a rebase in
// progress after an exit-code-one failure, it aborts the rebase and returns errRebaseFailed.
//
// @param           repoConfig  "configuration for the repository to rebase"
//
// @return          error       "nil on success, errRebaseFailed after an aborted conflict, or another error"
func rebase(repoConfig RepoConfig) error {
	repoPath := repoConfig.RepoPath
	bi, err := fetchBranchInfo(repoPath)
	if err != nil {
		return tracerr.Wrap(err)
	}

	_, rebaseErr := GitCommand(repoConfig, []string{"rebase", bi.UpstreamRemote + "/" + bi.UpstreamBranch})
	if rebaseErr != nil {
		rebaseInProgress, err := isRebasing(repoPath)
		if err != nil {
			return tracerr.Wrap(err)
		}

		var exerr *exec.ExitError
		if errors.As(rebaseErr, &exerr) && exerr.ExitCode() == 1 && rebaseInProgress {
			_, err := GitCommand(repoConfig, []string{"rebase", "--abort"})
			if err != nil {
				return tracerr.Wrap(err)
			}
			return errRebaseFailed
		}
		return tracerr.Wrap(rebaseErr)
	}

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
func exists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

type branchInfo struct {
	CurrentBranch  string
	UpstreamRemote string
	UpstreamBranch string
}

// @description    Reads current branch and upstream details.
//
// fetchBranchInfo reads the current branch and its configured upstream remote and branch,
// returning only the current branch when no tracking entry exists.
//
// @param           repoPath    "path to the repository root"
//
// @return          branchInfo  "current branch and any configured upstream details"
//
// @return          error       "nil on success, or an error reading the repository, config, or HEAD"
func fetchBranchInfo(repoPath string) (branchInfo, error) {
	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return branchInfo{}, tracerr.Wrap(err)
	}

	config, err := repo.Config()
	if err != nil {
		return branchInfo{}, tracerr.Wrap(err)
	}

	ref, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return branchInfo{}, tracerr.Wrap(err)
	}

	currentBranchName := ref.Target().Short()
	branchConfig := config.Branches[currentBranchName]
	if branchConfig == nil {
		// No tracking branch, nothing to do
		return branchInfo{CurrentBranch: currentBranchName}, nil
	}

	return branchInfo{
		CurrentBranch:  currentBranchName,
		UpstreamRemote: branchConfig.Remote,
		UpstreamBranch: branchConfig.Merge.Short(),
	}, nil
}

// @description    isRebasing checks for Git's rebase-apply and rebase-merge state directories.
//
// @param           repoPath  "path to the repository root"
//
// @return          bool      "true when either rebase state directory exists"
//
// @return          error     "nil on success, or an error inspecting a state directory"
func isRebasing(repoPath string) (bool, error) {
	ra, err := exists(path.Join(repoPath, ".git", "rebase-apply"))
	if err != nil {
		return false, tracerr.Wrap(err)
	}

	rm, err := exists(path.Join(repoPath, ".git", "rebase-merge"))
	if err != nil {
		return false, tracerr.Wrap(err)
	}

	return ra || rm, nil
}
