package common

import (
	"errors"
	"os/exec"

	"github.com/ztrue/tracerr"
)

var errNoGitAuthorEmail = errors.New("missing git author email")
var errNoGitAuthorName = errors.New("missing git author name")

// @description    Validates Git author identity.
//
// ensureGitAuthor verifies that Git user.email and user.name are set, returning a specific
// missing-author error or the underlying command error.
//
// @param           repoConfig  "configuration for the repository to inspect"
//
// @return          error       "nil when both author values exist, or an author or command error"
func ensureGitAuthor(repoConfig RepoConfig) error {
	_, err := GitCommand(repoConfig, []string{"config", "user.email"})
	if err != nil {
		var exerr *exec.ExitError
		if errors.As(err, &exerr) && exerr.ExitCode() == 1 {
			return errNoGitAuthorEmail
		}
		return tracerr.Wrap(err)
	}

	_, err = GitCommand(repoConfig, []string{"config", "user.name"})
	if err != nil {
		var exerr *exec.ExitError
		if errors.As(err, &exerr) && exerr.ExitCode() == 1 {
			return errNoGitAuthorName
		}
		return tracerr.Wrap(err)
	}

	return nil
}
