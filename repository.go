package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/ztrue/tracerr"
)

var errRepoPathInvalid = errors.New("not a valid git repo")

// @description    Validates a Git worktree path.
//
// isValidGitRepo verifies that a caller-provided path belongs to a non-bare Git worktree and walks
// upward to find the repository root containing a .git directory.
//
// @param           repoPath  "caller-provided path to validate as a Git repository or descendant"
//
// @return          string    "repository root derived from the caller-provided path"
//
// @return          error     "nil on success, or an error for an invalid path or repository"
func isValidGitRepo(repoPath string) (string, error) {
	info, err := os.Stat(repoPath)
	if os.IsNotExist(err) {
		return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
	}

	if !info.IsDir() {
		return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
	}

	_, err = git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", tracerr.Errorf("Not a valid git repo - %s\n%w", repoPath, err)
	}

	for {
		info, err := os.Stat(filepath.Join(repoPath, ".git"))
		if err != nil {
			if !os.IsNotExist(err) {
				return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
			}
		}

		if os.IsNotExist(err) {
			repoPath = filepath.Dir(repoPath)
			continue
		}

		if !info.IsDir() {
			return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
		}
		break
	}

	return repoPath, nil
}
