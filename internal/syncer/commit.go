package syncer

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
)

// @description    Commits eligible worktree changes.
//
// commit reads the working-tree status with git status, skips nested Git repositories and ignored
// files, stages eligible paths with git add, sorts their status lines, and creates a commit with
// those lines as its message. It logs a skip when no eligible changes exist.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to commit"
//
// @return          error       "nil on success or no eligible changes, or a repository or Git error"
func commit(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting commit")

	repoPath := repoConfig.RepoPath

	statusOut, err := gitCommand(logger, repoConfig, []string{"status", "--porcelain", "-z", "--no-renames", "--untracked-files=all"})
	if err != nil {
		logger.Error("commit failed", "operation", "read status", "error", err)
		return tracerr.Wrap(err)
	}

	hasChanges := false
	commitMsg := []string{}
	for _, record := range strings.Split(statusOut.String(), "\x00") {
		if len(record) < 4 {
			continue
		}

		// Porcelain record format is "XY path": two status columns, a space, then the path.
		filePath := record[3:]

		if isNestedGitRepository(repoPath, filePath) {
			logger.Debug("commit skipping nested git repository", "path", filePath)
			continue
		}

		ignore, err := ShouldIgnoreFile(repoPath, filePath)
		if err != nil {
			logger.Error("commit failed", "operation", "check ignored file", "path", filePath, "error", err)
			return tracerr.Wrap(err)
		}
		if ignore {
			continue
		}

		hasChanges = true
		if _, err := gitCommand(logger, repoConfig, []string{"add", "--", filePath}); err != nil {
			logger.Error("commit failed", "operation", "stage file", "path", filePath, "error", err)
			return tracerr.Wrap(err)
		}

		commitMsg = append(commitMsg, record)
	}

	sort.Strings(commitMsg)
	msg := strings.Join(commitMsg, "\n")

	if !hasChanges {
		logger.Info("commit skipped", "reason", "no eligible changes")
		return nil
	}

	if _, err := gitCommand(logger, repoConfig, []string{"commit", "-m", msg}); err != nil {
		logger.Error("commit failed", "operation", "create commit", "error", err)
		return tracerr.Wrap(err)
	}

	logger.Info("commit completed", "msgs", strings.Join(commitMsg, " "))
	return nil
}

// @description    Reports whether a path is a nested Git repository.
//
// isNestedGitRepository reports whether the repository-relative path resolves to a directory that
// is itself a Git repository, identified by a .git file or directory. The commit loop uses this to
// avoid staging an embedded repository as a gitlink.
//
// @param           repoPath  "path to the repository root"
//
// @param           relPath   "repository-relative path from git status, possibly with a trailing slash"
//
// @return          bool      "true when the path is a directory containing a .git entry"
func isNestedGitRepository(repoPath string, relPath string) bool {
	relPath = strings.TrimSuffix(relPath, "/")
	if relPath == "" {
		return false
	}

	absPath := filepath.Join(repoPath, relPath)
	info, err := os.Stat(absPath)
	if err != nil || !info.IsDir() {
		return false
	}

	_, err = os.Stat(filepath.Join(absPath, ".git"))
	return err == nil
}
