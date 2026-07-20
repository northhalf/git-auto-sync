package syncer

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/northhalf/git-auto-sync/internal/config"
)

// upstreamState describes how HEAD relates to its configured upstream after fetch has refreshed the
// remote-tracking reference.
type upstreamState string

const (
	// upstreamStateNone is returned when the current branch has no configured upstream.
	upstreamStateNone upstreamState = "none"
	// upstreamStateEqual is returned when HEAD matches the upstream tip.
	upstreamStateEqual upstreamState = "equal"
	// upstreamStateLocalAhead is returned when only HEAD has commits beyond the upstream tip.
	upstreamStateLocalAhead upstreamState = "local-ahead"
	// upstreamStateUpstreamAhead is returned when only the upstream has commits beyond HEAD.
	upstreamStateUpstreamAhead upstreamState = "upstream-ahead"
	// upstreamStateDiverged is returned when both HEAD and the upstream have commits beyond their
	// merge-base.
	upstreamStateDiverged upstreamState = "diverged"
)

// @description    Resolves the synchronization state between HEAD and its upstream.
//
// resolveUpstreamState compares the current HEAD against its configured upstream remote-tracking
// branch after fetch has updated it. It returns a state that AutoSync uses to decide whether to
// rebase and push: none when no upstream is configured, equal when HEAD matches the upstream,
// local-ahead when only HEAD has new commits, upstream-ahead when only the upstream has new
// commits, and diverged when both sides have new commits. The comparison uses git rev-parse and
// git merge-base on the upstream ref, so it never relies on a non-zero exit code to distinguish
// ancestry.
//
// @param           logger        "repository-scoped logger"
//
// @param           repoConfig    "configuration for the repository to inspect"
//
// @return          upstreamState "resolved synchronization state"
//
// @return          error         "nil on success, or an error reading branch info or Git refs"
func resolveUpstreamState(logger *slog.Logger, repoConfig config.RepoConfig) (upstreamState, error) {
	bi, err := fetchBranchInfo(repoConfig.RepoPath)
	if err != nil {
		logger.Error("resolve upstream state failed", "operation", "read branch information", "error", err)
		return "", err
	}

	if bi.UpstreamRemote == "" || bi.UpstreamBranch == "" {
		return upstreamStateNone, nil
	}

	ref := bi.UpstreamRemote + "/" + bi.UpstreamBranch

	revOut, err := gitCommand(logger, repoConfig, []string{"rev-parse", "HEAD", ref})
	if err != nil {
		logger.Error("resolve upstream state failed", "operation", "resolve refs", "ref", ref, "error", err)
		return "", err
	}

	lines := strings.Split(strings.TrimSpace(revOut.String()), "\n")
	if len(lines) < 2 {
		logger.Error("resolve upstream state failed", "operation", "parse refs", "ref", ref)
		return "", fmt.Errorf("unexpected rev-parse output for %s", ref)
	}
	headSha := strings.TrimSpace(lines[0])
	upstreamSha := strings.TrimSpace(lines[1])

	if headSha == upstreamSha {
		return upstreamStateEqual, nil
	}

	baseOut, err := gitCommand(logger, repoConfig, []string{"merge-base", "HEAD", ref})
	if err != nil {
		logger.Error("resolve upstream state failed", "operation", "merge-base", "ref", ref, "error", err)
		return "", err
	}
	baseSha := strings.TrimSpace(baseOut.String())

	switch baseSha {
	case headSha:
		return upstreamStateUpstreamAhead, nil
	case upstreamSha:
		return upstreamStateLocalAhead, nil
	default:
		return upstreamStateDiverged, nil
	}
}
