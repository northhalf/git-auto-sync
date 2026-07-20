package syncer

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

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
		return branchInfo{}, err
	}

	config, err := repo.Config()
	if err != nil {
		return branchInfo{}, err
	}

	ref, err := repo.Reference(plumbing.HEAD, false)
	if err != nil {
		return branchInfo{}, err
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
