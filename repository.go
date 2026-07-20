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
// isValidGitRepo verifies that a caller-provided path belongs to a non-bare Git worktree and
// resolves the repository root through go-git's upward .git detection. A repository whose .git is
// a file rather than a directory (a linked worktree or submodule) is rejected because the daemon's
// .git/config polling requires a real .git directory.
//
// @param           repoPath  "caller-provided path to validate as a Git repository or descendant"
//
// @return          string    "repository root derived from the caller-provided path"
//
// @return          error     "nil on success, or an error for an invalid path or repository"
func isValidGitRepo(repoPath string) (string, error) {
	info, err := os.Stat(repoPath)
	if err != nil || !info.IsDir() {
		return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
	}

	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", tracerr.Errorf("Not a valid git repo - %s\n%w", repoPath, err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return "", tracerr.Errorf("Not a valid git repo - %s\n%w", repoPath, err)
	}

	root := worktree.Filesystem.Root()
	info, err = os.Stat(filepath.Join(root, ".git"))
	if err != nil || !info.IsDir() {
		return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
	}

	return root, nil
}
