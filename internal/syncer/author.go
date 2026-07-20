package syncer

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/northhalf/git-auto-sync/internal/config"
)

var errNoGitAuthorEmail = errors.New("missing git author email")
var errNoGitAuthorName = errors.New("missing git author name")

// authorEnvMutex serializes author config resolution because go-git loads
// configuration from the current process environment.
var authorEnvMutex sync.Mutex

// @description    Validates Git author identity.
//
// ensureGitAuthor verifies that Git user.email and user.name are set without logging either value,
// returning a specific missing-author error or the underlying repository error. It merges system,
// global, and local configuration the same way `git config` does, applying HOME and XDG_CONFIG_HOME
// from repoConfig.Env when resolving global config. Empty values are treated as missing.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to inspect"
//
// @return          error       "nil when both author values exist, or an author or repository error"
func ensureGitAuthor(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting git author verification")

	repo, err := git.PlainOpenWithOptions(repoConfig.RepoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		logger.Error("git author verification failed", "operation", "open repository", "error", err)
		return err
	}

	envMap := envMapFromSlice(repoConfig.Env)

	authorEnvMutex.Lock()
	defer authorEnvMutex.Unlock()

	// Replace system environment variables with repository environment variables
	restore := setAuthorEnv(logger, envMap)
	defer func() {
		if err := restore(); err != nil {
			logger.Warn("failed to restore git author environment", "error", err)
		}
	}()

	cfg, err := repo.ConfigScoped(gitconfig.SystemScope)
	if err != nil {
		logger.Error("git author verification failed", "operation", "read config", "error", err)
		return err
	}

	if cfg.User.Email == "" {
		logger.Error("git author verification failed", "field", "user.email", "error", errNoGitAuthorEmail)
		return errNoGitAuthorEmail
	}

	if cfg.User.Name == "" {
		logger.Error("git author verification failed", "field", "user.name", "error", errNoGitAuthorName)
		return errNoGitAuthorName
	}

	logger.Info("git author verified")
	return nil
}

// @description    Parses environment entries into a map.
//
// envMapFromSlice converts a slice of KEY=VALUE entries into a map. The last occurrence wins
// when a key appears multiple times. Entries without an '=' separator are stored with an empty
// value.
//
// @param           env     "environment entries in key=value form"
//
// @return          map[string]string  "parsed environment map"
func envMapFromSlice(env []string) map[string]string {
	m := make(map[string]string)
	for _, s := range env {
		k, v, _ := strings.Cut(s, "=")
		m[k] = v
	}
	return m
}

// @description    Temporarily applies effective author environment values.
//
// setAuthorEnv reads the current values of HOME, XDG_CONFIG_HOME, and GIT_CONFIG_GLOBAL, then
// replaces them with the values from envMap. Variables present in envMap use that value (empty
// values unset the variable); variables not present keep their current process value. The
// returned function restores the original values and must be called after the caller finishes
// loading configuration from the process environment.
//
// @param           logger  "repository-scoped logger for environment mutation failures"
//
// @param           envMap  "environment overrides from repository configuration"
//
// @return          func() error  "function that restores the original environment values; returns any restoration error"
func setAuthorEnv(logger *slog.Logger, envMap map[string]string) func() error {
	vars := []string{"HOME", "XDG_CONFIG_HOME", "GIT_CONFIG_GLOBAL"}
	type envState struct {
		value string
		set   bool
	}
	original := make(map[string]envState, len(vars))

	for _, v := range vars {
		if val, ok := os.LookupEnv(v); ok {
			original[v] = envState{value: val, set: true}
		} else {
			original[v] = envState{value: "", set: false}
		}

		if explicit, ok := envMap[v]; ok {
			if explicit == "" {
				if err := os.Unsetenv(v); err != nil {
					logger.Warn("failed to unset author config env", "key", v, "error", err)
				}
			} else {
				if err := os.Setenv(v, explicit); err != nil {
					logger.Warn("failed to set author config env", "key", v, "error", err)
				}
			}
		}
	}

	return func() error {
		var errs []error
		for _, v := range vars {
			if orig, ok := original[v]; ok && orig.set {
				if err := os.Setenv(v, orig.value); err != nil {
					errs = append(errs, err)
				}
			} else {
				if err := os.Unsetenv(v); err != nil {
					errs = append(errs, err)
				}
			}
		}
		return errors.Join(errs...)
	}
}
