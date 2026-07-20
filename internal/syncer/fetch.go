package syncer

import (
	"log/slog"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
)

// @description    Fetches the current branch's configured upstream branch.
//
// fetch runs Git fetch for only the upstream remote and branch of the current branch, the single
// remote-tracking reference that the compare and rebase stages read. It skips repositories without
// a configured upstream and branches tracking a local branch (remote "."), neither of which needs
// a network operation. A fetch failure, such as an upstream branch deleted on the remote, stops
// the pipeline at the fetch stage. Remote names may be logged, but remote URLs are never logged.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to fetch"
//
// @return          error       "nil when the upstream branch is fetched or no fetch is needed, or the fetch error"
func fetch(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting fetch")

	bi, err := fetchBranchInfo(repoConfig.RepoPath)
	if err != nil {
		logger.Error("fetch failed", "operation", "read branch information", "error", err)
		return tracerr.Wrap(err)
	}

	if bi.UpstreamRemote == "" || bi.UpstreamBranch == "" {
		logger.Info("fetch skipped", "reason", "no upstream")
		return nil
	}

	if bi.UpstreamRemote == "." {
		logger.Info("fetch skipped", "reason", "local upstream", "branch", bi.UpstreamBranch)
		return nil
	}

	if _, err := gitCommand(logger, repoConfig, []string{"fetch", bi.UpstreamRemote, bi.UpstreamBranch}); err != nil {
		logger.Error("fetch failed", "remote", bi.UpstreamRemote, "branch", bi.UpstreamBranch, "error", err)
		return tracerr.Wrap(err)
	}

	logger.Info("fetch completed", "remote", bi.UpstreamRemote, "branch", bi.UpstreamBranch)
	return nil
}
