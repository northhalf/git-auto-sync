package syncer

import (
	"log/slog"

	"github.com/northhalf/git-auto-sync/internal/config"
)

// @description    Pushes to the configured upstream.
//
// push runs git push with the configured upstream remote and upstream branch as arguments, or
// logs a skip without running Git when either is unset. It never logs the remote URL.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to push"
//
// @return          error       "nil on success or no upstream, or a branch lookup or Git error"
func push(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting push")

	bi, err := fetchBranchInfo(repoConfig.RepoPath)
	if err != nil {
		logger.Error("push failed", "operation", "read branch information", "error", err)
		return err
	}

	if bi.UpstreamBranch == "" || bi.UpstreamRemote == "" {
		logger.Info("push skipped", "reason", "no upstream")
		return nil
	}

	if _, err := gitCommand(logger, repoConfig, []string{"push", bi.UpstreamRemote, bi.UpstreamBranch}); err != nil {
		logger.Error("push failed", "remote", bi.UpstreamRemote, "branch", bi.UpstreamBranch, "error", err)
		return err
	}

	logger.Info("push completed", "remote", bi.UpstreamRemote, "branch", bi.UpstreamBranch)
	return nil
}
