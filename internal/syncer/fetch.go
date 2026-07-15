package syncer

import (
	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
	git "gopkg.in/src-d/go-git.v4"
)

// @description    Fetches every configured remote.
//
// fetch runs Git fetch for every remote configured in the repository, stopping at the first
// repository or command error.
//
// @param           repoConfig  "configuration for the repository to fetch"
//
// @return          error       "nil when all remotes are fetched, or the first encountered error"
func fetch(repoConfig config.RepoConfig) error {
	repoPath := repoConfig.RepoPath
	r, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return tracerr.Wrap(err)
	}

	remotes, err := r.Remotes()
	if err != nil {
		return tracerr.Wrap(err)
	}

	for _, remote := range remotes {
		remoteName := remote.Config().Name

		_, err := gitCommand(repoConfig, []string{"fetch", remoteName})
		if err != nil {
			return tracerr.Wrap(err)
		}
	}

	return nil
}
