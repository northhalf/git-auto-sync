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

// @description    Caches a repository's Git index and ignore rules for repeated ignore checks.
//
// IgnoreChecker opens the repository, reads the Git index, and compiles gitignore patterns once
// at construction, then performs no further repository IO on subsequent ShouldIgnore calls.
type IgnoreChecker struct {
	repoPath string
	tracked  map[string]struct{}
	matcher  gitignore.Matcher
}

// @description    Builds an IgnoreChecker from a repository.
//
// NewIgnoreChecker opens the repo once, reads the index once into a tracked-path set, and parses
// ignore patterns once into a compiled matcher. The returned checker reuses this snapshot for
// every ShouldIgnore call.
//
// @param           repoPath          "path to the repository root"
//
// @return          *IgnoreChecker    "checker with the cached tracked set and compiled matcher"
//
// @return          error             "nil on success, or an error opening the repository or reading the index or ignore rules"
func NewIgnoreChecker(repoPath string) (*IgnoreChecker, error) {
	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	index, err := repo.Storer.Index()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	tracked := make(map[string]struct{}, len(index.Entries))
	for _, entry := range index.Entries {
		tracked[filepath.ToSlash(entry.Name)] = struct{}{}
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	patterns, err := gitignore.ReadPatterns(w.Filesystem, nil)
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	patterns = append(patterns, w.Excludes...)
	matcher := gitignore.NewMatcher(patterns)

	return &IgnoreChecker{
		repoPath: repoPath,
		tracked:  tracked,
		matcher:  matcher,
	}, nil
}

// @description    Reports whether a file should be ignored.
//
// ShouldIgnore reuses the cached index and matcher. Tracked files bypass every check. Untracked
// paths are excluded when they are editor temporary files, untracked hidden paths, Git metadata,
// empty files, or matched by Git ignore/exclude rules. Git control files, .github contents, and
// files ending in .example remain eligible for the remaining checks.
//
// @param           filePath  "absolute or repository-relative path within the repository to inspect"
//
// @return          bool      "true when the file should be excluded from watching and commits"
//
// @return          error     "nil on success, or an error while inspecting the file"
func (c *IgnoreChecker) ShouldIgnore(filePath string) (bool, error) {
	if strings.TrimSpace(filePath) == "" {
		return false, errors.New("file path cannot be empty")
	}

	if !filepath.IsAbs(filePath) {
		filePath = path.Join(c.repoPath, filePath)
	}

	relativePath, err := filepath.Rel(c.repoPath, filePath)
	if err != nil {
		return false, tracerr.Wrap(err)
	}
	relativePath = filepath.ToSlash(relativePath)

	if _, ok := c.tracked[relativePath]; ok {
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
		fileName == ".gitkeep" ||
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
	if empty && fileName != ".gitkeep" {
		return true, nil
	}

	if relativePath == ".." || strings.HasPrefix(relativePath, "../") {
		return false, errors.New("file path is outside repository")
	}
	if relativePath == "." {
		return false, nil
	}

	return c.matcher.Match(strings.Split(relativePath, "/"), false), nil
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
