package syncer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// @description    Caches a repository's Git index and ignore rules for repeated ignore checks.
//
// IgnoreChecker opens the repository, reads the Git index, and compiles gitignore patterns once
// at construction, then performs no further repository IO on subsequent ShouldIgnore calls.
type IgnoreChecker struct {
	repoPath         string
	resolvedRepoPath string
	tracked          map[string]struct{}
	matcher          gitignore.Matcher
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
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}
	repoPath = filepath.Clean(repoPath)

	resolvedRepoPath, err := filepath.EvalSymlinks(repoPath)
	if err != nil {
		return nil, err
	}

	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return nil, err
	}

	index, err := repo.Storer.Index()
	if err != nil {
		return nil, err
	}

	tracked := make(map[string]struct{}, len(index.Entries))
	for _, entry := range index.Entries {
		tracked[filepath.ToSlash(entry.Name)] = struct{}{}
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	patterns, err := gitignore.ReadPatterns(w.Filesystem, nil)
	if err != nil {
		return nil, err
	}
	patterns = append(patterns, w.Excludes...)
	matcher := gitignore.NewMatcher(patterns)

	return &IgnoreChecker{
		repoPath:         repoPath,
		resolvedRepoPath: resolvedRepoPath,
		tracked:          tracked,
		matcher:          matcher,
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

	repoPath, filePath, relativePath, err := c.resolvePath(filePath)
	if err != nil {
		return false, err
	}

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

	// A Windows-hidden directory or file is an explicit user action, so untracked paths under one
	// are ignored before the dot-prefix exceptions below can exempt them. Tracked files already
	// bypassed above remain eligible.
	if isHiddenByOS(repoPath, filePath) {
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
		return false, err
	}
	if empty && fileName != ".gitkeep" {
		return true, nil
	}

	if outsideRepo {
		return false, errors.New("file path is outside repository")
	}
	if relativePath == "." {
		return false, nil
	}

	return c.matcher.Match(strings.Split(relativePath, "/"), false), nil
}

// @description    Resolves a file path against the configured and physical repository roots.
//
// resolvePath first accepts paths under the configured repository root, then paths under the root
// after symbolic links are resolved. If neither matches, it resolves the file path through its
// nearest existing ancestor so deleted and renamed events can still map through a symbolic link.
// Paths that remain outside are returned relative to the configured root so ShouldIgnore preserves
// its existing filtering order before reporting the boundary error.
//
// @param           filePath     "absolute or repository-relative path to resolve"
//
// @return          matchedRepoPath   string  "repository root matching the resolved file path"
//
// @return          resolvedFilePath  string  "absolute file path in the matching root's path space"
//
// @return          relativePath      string  "slash-separated repository-relative path"
//
// @return          err               error   "path resolution or relative-path error"
func (c *IgnoreChecker) resolvePath(filePath string) (
	matchedRepoPath string,
	resolvedFilePath string,
	relativePath string,
	err error,
) {
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(c.repoPath, filePath)
	}
	filePath = filepath.Clean(filePath)

	originalRel, originalInside, originalErr := relativeInsideRepo(c.repoPath, filePath)
	if originalErr == nil && originalInside {
		return c.repoPath, filePath, originalRel, nil
	}

	resolvedRel, resolvedInside, resolvedErr := relativeInsideRepo(c.resolvedRepoPath, filePath)
	if resolvedErr == nil && resolvedInside {
		return c.resolvedRepoPath, filePath, resolvedRel, nil
	}

	resolvedFilePath, err = resolveExistingPath(filePath)
	if err != nil {
		return "", "", "", err
	}
	resolvedRel, resolvedInside, resolvedErr = relativeInsideRepo(c.resolvedRepoPath, resolvedFilePath)
	if resolvedErr == nil && resolvedInside {
		return c.resolvedRepoPath, resolvedFilePath, resolvedRel, nil
	}

	if originalErr != nil {
		return "", "", "", originalErr
	}
	return c.repoPath, filePath, originalRel, nil
}

// @description    Computes a repository-relative path and whether it stays within the root.
//
// @param           repoPath  "absolute repository root"
//
// @param           filePath  "absolute file path to compare"
//
// @return          string    "slash-separated path relative to repoPath"
//
// @return          bool      "true when filePath is repoPath or one of its descendants"
//
// @return          error     "filepath.Rel error, including incompatible Windows volumes"
func relativeInsideRepo(repoPath string, filePath string) (string, bool, error) {
	relativePath, err := filepath.Rel(repoPath, filePath)
	if err != nil {
		return "", false, err
	}
	relativePath = filepath.ToSlash(relativePath)
	outside := relativePath == ".." || strings.HasPrefix(relativePath, "../")
	return relativePath, !outside, nil
}

// @description    Resolves symbolic links while preserving a missing path suffix.
//
// resolveExistingPath walks toward the filesystem root until filepath.EvalSymlinks finds an
// existing ancestor, then appends the removed suffix. This supports file removal and rename events
// whose reported target no longer exists.
//
// @param           filePath  "absolute path to resolve"
//
// @return          string    "resolved absolute path with any missing suffix restored"
//
// @return          error     "non-absence filesystem error or failure to find an existing ancestor"
func resolveExistingPath(filePath string) (string, error) {
	candidate := filePath
	missingParts := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil {
			for i := len(missingParts) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missingParts[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", err
		}
		missingParts = append(missingParts, filepath.Base(candidate))
		candidate = parent
	}
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
