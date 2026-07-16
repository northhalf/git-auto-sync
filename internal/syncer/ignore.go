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
// ShouldIgnoreFile reports whether a path should be excluded from watching and commits. Files tracked
// in the Git index are always eligible and bypass every other check. Untracked paths are excluded
// when they are editor temporary files, untracked hidden paths, Git metadata, empty files, or matched
// by Git ignore and exclude rules. Git control files, .github contents, and files ending in .example
// remain eligible for the remaining checks.
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

	relativePath, err := filepath.Rel(repoPath, filePath)
	if err != nil {
		return false, tracerr.Wrap(err)
	}
	relativePath = filepath.ToSlash(relativePath)

	tracked, err := isTracked(repoPath, relativePath)
	if err != nil {
		return false, tracerr.Wrap(err)
	}
	if tracked {
		return false, nil
	}

	fileName := filepath.Base(filePath)
	var isTempFile = strings.HasSuffix(fileName, ".swp") || // vim
		strings.HasPrefix(fileName, "~") || // emacs
		strings.HasSuffix(fileName, "~") // kate

	if isTempFile {
		return true, nil
	}

	if strings.HasPrefix(relativePath, ".git/") {
		return true, nil
	}

	pathParts := strings.Split(relativePath, "/")
	hidden := false
	for _, part := range pathParts {
		if part != "." && part != ".." && strings.HasPrefix(part, ".") {
			hidden = true
			break
		}
	}

	hiddenException := pathParts[0] == ".github" ||
		fileName == ".gitignore" ||
		fileName == ".gitattributes" ||
		fileName == ".gitmodules" ||
		fileName == ".mailmap" ||
		strings.HasSuffix(fileName, ".example")
	outsideRepo := relativePath == ".." || strings.HasPrefix(relativePath, "../")
	if hidden && !hiddenException && !outsideRepo {
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

// @description    Reports whether a path is tracked in the Git index.
//
// isTracked opens the repository and searches the index for the repository-relative path. The path
// must already use forward slashes, as produced by filepath.ToSlash.
//
// @param           repoPath      "path to the repository root"
//
// @param           relativePath  "repository-relative path using forward slashes"
//
// @return          bool          "true when an index entry matches the path"
//
// @return          error         "nil on success, or an error opening the repository or reading the index"
func isTracked(repoPath string, relativePath string) (bool, error) {
	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return false, tracerr.Wrap(err)
	}

	index, err := repo.Storer.Index()
	if err != nil {
		return false, tracerr.Wrap(err)
	}

	for _, entry := range index.Entries {
		if entry.Name == relativePath {
			return true, nil
		}
	}
	return false, nil
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
