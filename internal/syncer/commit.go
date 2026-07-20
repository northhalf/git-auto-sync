package syncer

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/northhalf/git-auto-sync/internal/config"
)

// @description    Commits eligible worktree changes.
//
// commit reads the working-tree status with git status, skips nested Git repositories and ignored
// files, stages eligible paths with git add, sorts their status lines, and creates a commit with
// those lines as its message. It logs a skip when no eligible changes exist, or when staging
// produced no index changes because a content filter (such as Git LFS with a pointer-only working
// tree under GIT_LFS_SKIP_SMUDGE) made git status report a change that git add did not stage.
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
		return err
	}

	checker, err := NewIgnoreChecker(repoPath)
	if err != nil {
		logger.Error("commit failed", "operation", "build ignore checker", "error", err)
		return err
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

		ignore, err := checker.ShouldIgnore(filePath)
		if err != nil {
			logger.Error("commit failed", "operation", "check ignored file", "path", filePath, "error", err)
			return err
		}
		if ignore {
			continue
		}

		hasChanges = true
		if _, err := gitCommand(logger, repoConfig, []string{"add", "--", filePath}); err != nil {
			logger.Error("commit failed", "operation", "stage file", "path", filePath, "error", err)
			return err
		}

		commitMsg = append(commitMsg, record)
	}

	sort.Strings(commitMsg)
	msg := strings.Join(commitMsg, "\n")

	if !hasChanges {
		logger.Info("commit skipped", "reason", "no eligible changes")
		return nil
	}

	// Confirm that staging actually wrote changes to the index before committing. Content filters
	// can make git status report a file as modified while git add stages nothing: a Git LFS file
	// left as a pointer in the working tree (for example under GIT_LFS_SKIP_SMUDGE or a CI checkout
	// without an LFS fetch) reports as modified, but git add produces no staged change and git
	// commit would then fail with "nothing to commit". Re-checking the staged index turns that
	// failure into a clean skip.
	stagedOut, err := gitCommand(logger, repoConfig, []string{"diff", "--cached", "--name-only", "-z"})
	if err != nil {
		logger.Error("commit failed", "operation", "check staged changes", "error", err)
		return err
	}
	if len(stagedOut.String()) == 0 {
		logger.Info("commit skipped", "reason", "no staged changes")
		return nil
	}

	if _, err := gitCommand(logger, repoConfig, []string{"commit", "-m", msg}); err != nil {
		logger.Error("commit failed", "operation", "create commit", "error", err)
		return err
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
