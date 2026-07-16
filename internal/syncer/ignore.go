package syncer

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"github.com/ztrue/tracerr"
)

// @description    Determines whether a file should be ignored.
//
// ShouldIgnoreFile reports whether a path is an editor temporary file, Git metadata, an empty
// file, or matched by Git ignore and exclude rules.
//
// @param           repoPath  "path to the repository root"
//
// @param           filePath  "absolute or repository-relative path within repoPath to inspect"
//
// @return          bool      "true when the file should be excluded from watching and commits"
//
// @return          error     "nil on success, or an error while inspecting the file or Git rules"
func ShouldIgnoreFile(repoPath string, filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, errors.New("file path cannot be empty")
	}

	if !filepath.IsAbs(filePath) {
		filePath = path.Join(repoPath, filePath)
	}

	fileName := filepath.Base(filePath)
	var isTempFile = strings.HasSuffix(fileName, ".swp") || // vim
		strings.HasPrefix(fileName, "~") || // emacs
		strings.HasSuffix(fileName, "~") // kate

	if isTempFile {
		return true, nil
	}

	relativePath, err := filepath.Rel(repoPath, filePath)
	if err != nil {
		return false, tracerr.Wrap(err)
	}
	relativePath = filepath.ToSlash(relativePath)

	if strings.HasPrefix(relativePath, ".git/") {
		return true, nil
	}

	empty, err := isEmptyFile(filePath)
	if err != nil {
		return false, tracerr.Wrap(err)
	}
	if empty {
		return true, nil
	}

	return isFileIgnoredByGit(repoPath, filePath)
}

// @description    Matches a file against Git ignore rules.
//
// isFileIgnoredByGit checks a path against repository ignore patterns and worktree exclude rules.
// Absolute and repository-relative paths are normalized into matcher path components. Paths outside
// the repository are rejected.
//
// @param           repoPath  "path to the repository root"
//
// @param           filePath  "absolute or repository-relative path to match against Git ignore rules"
//
// @return          bool      "true when a Git ignore or exclude rule matches the path"
//
// @return          error     "nil on success, or an error normalizing the path or reading Git rules"
func isFileIgnoredByGit(repoPath string, filePath string) (bool, error) {
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(repoPath, filePath)
	}

	relativePath, err := filepath.Rel(repoPath, filePath)
	if err != nil {
		return false, tracerr.Wrap(err)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return false, errors.New("file path is outside repository")
	}
	if relativePath == "." {
		return false, nil
	}

	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return false, tracerr.Wrap(err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return false, tracerr.Wrap(err)
	}

	patterns, err := gitignore.ReadPatterns(w.Filesystem, nil)
	if err != nil {
		return false, tracerr.Wrap(err)
	}

	patterns = append(patterns, w.Excludes...)
	m := gitignore.NewMatcher(patterns)

	relativePath = filepath.ToSlash(relativePath)
	return m.Match(strings.Split(relativePath, "/"), false), nil
}

// @description    Checks whether an existing file is empty.
//
// isEmptyFile reports whether an existing path has zero bytes and treats a missing path as non-
// empty so deletion events remain eligible.
//
// @param           filePath  "filesystem path to inspect"
//
// @return          bool      "true when the existing path has zero bytes"
//
// @return          error     "nil for existing or missing paths, or the filesystem error"
func isEmptyFile(filePath string) (bool, error) {
	stat, err := os.Stat(filePath)

	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return stat.Size() == 0, nil
}
