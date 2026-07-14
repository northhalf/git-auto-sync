package common

import (
	"github.com/ztrue/tracerr"
)

// @description    Pushes to the configured upstream.
//
// push runs git push with the configured upstream remote and upstream branch as arguments, or
// returns without running Git when either is unset.
//
// @param           repoConfig  "configuration for the repository to push"
//
// @return          error       "nil on success or no upstream, or a branch lookup or Git error"
func push(repoConfig RepoConfig) error {
	bi, err := fetchBranchInfo(repoConfig.RepoPath)
	if err != nil {
		return tracerr.Wrap(err)
	}

	if bi.UpstreamBranch == "" || bi.UpstreamRemote == "" {
		return nil
	}

	_, err = GitCommand(repoConfig, []string{"push", bi.UpstreamRemote, bi.UpstreamBranch})
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}
