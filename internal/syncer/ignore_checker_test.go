package syncer

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"gotest.tools/v3/assert"
)

// @description    Verifies IgnoreChecker matches ShouldIgnoreFile on existing cases.
//
// Test_IgnoreChecker_EquivalentToShouldIgnoreFile verifies that constructing one IgnoreChecker
// and querying many paths yields the same results as the package-level ShouldIgnoreFile.
//
// @param           t   "test handle used for fixture setup and equivalence assertions"
func Test_IgnoreChecker_EquivalentToShouldIgnoreFile(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath

	paths := []string{
		".hidden",
		filepath.Join(".cache", "file"),
		filepath.Join("src", ".cache", "file"),
		filepath.Join(repoPath, ".absolute"),
		filepath.Join("nested", ".gitignore"),
		filepath.Join("nested", ".gitattributes"),
		".gitmodules",
		".mailmap",
		filepath.Join(".github", "workflows", "ci.yml"),
		filepath.Join(".github", ".private", "config"),
		".env.example",
		filepath.Join("config", ".env.example"),
		repoPath,
		"",
	}

	checker, err := NewIgnoreChecker(repoPath)
	assert.NilError(t, err)

	for _, p := range paths {
		got, err := checker.ShouldIgnore(p)
		want, wantErr := ShouldIgnoreFile(repoPath, p)

		if p == "" {
			assert.ErrorContains(t, err, "path")
			assert.ErrorContains(t, wantErr, "path")
			continue
		}
		assert.NilError(t, err)
		assert.NilError(t, wantErr)
		assert.Equal(t, got, want, "mismatch for path %q", p)
	}
}

// @description    Verifies a single checker answers many queries in one round.
//
// Test_IgnoreChecker_RoundReuse verifies that one IgnoreChecker, built once, returns correct
// results across mixed tracked, untracked, and ignored paths without re-reading the index.
//
// @param           t   "test handle used to stage a tracked file and assert mixed-query results"
func Test_IgnoreChecker_RoundReuse(t *testing.T) {
	repoPath := t.TempDir()
	repo, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	worktree, err := repo.Worktree()
	assert.NilError(t, err)

	// A tracked file (staged) must bypass every filter.
	trackedPath := filepath.Join(repoPath, ".tracked")
	assert.NilError(t, os.WriteFile(trackedPath, []byte("tracked"), 0o600))
	_, err = worktree.Add(".tracked")
	assert.NilError(t, err)

	// An untracked hidden file must be ignored.
	hiddenPath := filepath.Join(repoPath, ".cache", "file")
	assert.NilError(t, os.MkdirAll(filepath.Dir(hiddenPath), 0o700))
	assert.NilError(t, os.WriteFile(hiddenPath, []byte("x"), 0o600))

	// An untracked non-hidden, non-ignored file must be eligible.
	plainPath := filepath.Join(repoPath, "plain.md")
	assert.NilError(t, os.WriteFile(plainPath, []byte("plain"), 0o600))

	checker, err := NewIgnoreChecker(repoPath)
	assert.NilError(t, err)

	ignore, err := checker.ShouldIgnore(trackedPath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)

	ignore, err = checker.ShouldIgnore(hiddenPath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, true)

	ignore, err = checker.ShouldIgnore(plainPath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)
}

// @description    Verifies staged files bypass filters and gitignored files are excluded.
//
// Test_IgnoreChecker_TrackedBypassesAndGitignoreExcludes verifies that the cached tracked map
// marks a staged file eligible even when it matches a gitignore rule, and marks an untracked
// gitignored file excluded.
//
// @param           t   "test handle used to stage files, write a gitignore, and assert results"
func Test_IgnoreChecker_TrackedBypassesAndGitignoreExcludes(t *testing.T) {
	repoPath := t.TempDir()
	repo, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	worktree, err := repo.Worktree()
	assert.NilError(t, err)

	trackedIgnoredPath := filepath.Join(repoPath, "ignored.txt")
	assert.NilError(t, os.WriteFile(trackedIgnoredPath, []byte("tracked"), 0o600))
	_, err = worktree.Add("ignored.txt")
	assert.NilError(t, err)

	untrackedIgnoredPath := filepath.Join(repoPath, "untracked.txt")
	assert.NilError(t, os.WriteFile(untrackedIgnoredPath, []byte("untracked"), 0o600))

	assert.NilError(t, os.WriteFile(filepath.Join(repoPath, ".gitignore"), []byte("*.txt\n"), 0o600))

	checker, err := NewIgnoreChecker(repoPath)
	assert.NilError(t, err)

	ignore, err := checker.ShouldIgnore(trackedIgnoredPath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)

	ignore, err = checker.ShouldIgnore(untrackedIgnoredPath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, true)
}

// @description    Verifies NewIgnoreChecker fails on a non-Git directory.
//
// Test_IgnoreChecker_NonGitDirectory verifies that constructing a checker for a directory without
// a Git repository returns an error.
//
// @param           t   "test handle used to create a temp dir and assert a construction error"
func Test_IgnoreChecker_NonGitDirectory(t *testing.T) {
	repoPath := t.TempDir()

	_, err := NewIgnoreChecker(repoPath)
	assert.Assert(t, err != nil)
}
