package syncer

import (
	"errors"
	"log/slog"
	"os/exec"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
)

var errNoGitAuthorEmail = errors.New("missing git author email")
var errNoGitAuthorName = errors.New("missing git author name")

// @description    Validates Git author identity.
//
// ensureGitAuthor verifies that Git user.email and user.name are set without logging either value,
// returning a specific missing-author error or the underlying command error.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to inspect"
//
// @return          error       "nil when both author values exist, or an author or command error"
func ensureGitAuthor(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting git author verification")

	_, err := gitCommand(logger, repoConfig, []string{"config", "user.email"})
	if err != nil {
		var exerr *exec.ExitError
		if errors.As(err, &exerr) && exerr.ExitCode() == 1 {
			logger.Error("git author verification failed", "field", "user.email", "error", errNoGitAuthorEmail)
			return errNoGitAuthorEmail
		}
		logger.Error("git author verification failed", "field", "user.email", "error", err)
		return tracerr.Wrap(err)
	}

	_, err = gitCommand(logger, repoConfig, []string{"config", "user.name"})
	if err != nil {
		var exerr *exec.ExitError
		if errors.As(err, &exerr) && exerr.ExitCode() == 1 {
			logger.Error("git author verification failed", "field", "user.name", "error", errNoGitAuthorName)
			return errNoGitAuthorName
		}
		logger.Error("git author verification failed", "field", "user.name", "error", err)
		return tracerr.Wrap(err)
	}

	logger.Info("git author verified")
	return nil
}
