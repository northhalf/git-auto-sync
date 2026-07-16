package syncer

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"gotest.tools/v3/assert"
)

// @description    Verifies basic Git ignore matching.
//
// Test_SimpleIgnore verifies that Git ignore rules classify 1.txt as ignored and 2.md as eligible
// for processing.
//
// @param           t   "test handle used for fixture setup and ignore assertions"
func Test_SimpleIgnore(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath

	ignore, err := isFileIgnoredByGit(repoPath, "1.txt")
	assert.NilError(t, err)
	assert.Equal(t, ignore, true)

	ignore, err = isFileIgnoredByGit(repoPath, "2.md")
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)
}

// @description    Verifies Git ignore matching for supported path forms.
//
// Test_IsFileIgnoredByGitPathForms verifies that absolute paths and nested repository-relative paths
// are converted into repository-relative components before matching ignore patterns.
//
// @param           t   "test handle used for fixture setup and path-form assertions"
func Test_IsFileIgnoredByGitPathForms(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath
	tests := []struct {
		name     string
		filePath string
	}{
		{name: "absolute", filePath: filepath.Join(repoPath, "ignored.txt")},
		{name: "nested relative", filePath: filepath.Join("nested", "ignored.txt")},
		{name: "nested absolute", filePath: filepath.Join(repoPath, "nested", "ignored.txt")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ignore, err := isFileIgnoredByGit(repoPath, tt.filePath)
			assert.NilError(t, err)
			assert.Equal(t, ignore, true)
		})
	}
}

// @description    Verifies rejection of paths outside the repository.
//
// Test_IsFileIgnoredByGitRejectsOutsidePaths verifies that relative and absolute paths resolving
// outside the repository return an error instead of being passed to the Git ignore matcher.
//
// @param           t   "test handle used for fixture setup and boundary assertions"
func Test_IsFileIgnoredByGitRejectsOutsidePaths(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath
	tests := []struct {
		name     string
		filePath string
	}{
		{name: "relative", filePath: filepath.Join("..", "outside.txt")},
		{name: "absolute", filePath: filepath.Join(filepath.Dir(repoPath), "outside.txt")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := isFileIgnoredByGit(repoPath, tt.filePath)
			assert.ErrorContains(t, err, "outside repository")
		})
	}
}

// @description    Verifies filtering of untracked hidden paths.
//
// Test_ShouldIgnoreFile_HiddenPaths verifies that root, nested, and absolute paths containing a
// dot-prefixed component are ignored when no exception applies.
//
// @param           t   "test handle used for fixture setup and hidden-path assertions"
func Test_ShouldIgnoreFile_HiddenPaths(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath
	tests := []struct {
		name     string
		filePath string
	}{
		{name: "root file", filePath: ".hidden"},
		{name: "root directory", filePath: filepath.Join(".cache", "file")},
		{name: "nested directory", filePath: filepath.Join("src", ".cache", "file")},
		{name: "absolute", filePath: filepath.Join(repoPath, ".absolute")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ignore, err := ShouldIgnoreFile(repoPath, tt.filePath)
			assert.NilError(t, err)
			assert.Equal(t, ignore, true)
		})
	}
}

// @description    Verifies exceptions to hidden-path filtering.
//
// Test_ShouldIgnoreFile_HiddenPathExceptions verifies that Git control files, .github contents,
// and files ending in .example remain eligible when no existing ignore rule excludes them.
//
// @param           t   "test handle used for fixture setup and hidden exception assertions"
func Test_ShouldIgnoreFile_HiddenPathExceptions(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath
	tests := []struct {
		name     string
		filePath string
	}{
		{name: "nested gitignore", filePath: filepath.Join("nested", ".gitignore")},
		{name: "nested gitattributes", filePath: filepath.Join("nested", ".gitattributes")},
		{name: "gitmodules", filePath: ".gitmodules"},
		{name: "mailmap", filePath: ".mailmap"},
		{name: "github content", filePath: filepath.Join(".github", "workflows", "ci.yml")},
		{name: "github hidden content", filePath: filepath.Join(".github", ".private", "config")},
		{name: "root example", filePath: ".env.example"},
		{name: "nested example", filePath: filepath.Join("config", ".env.example")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ignore, err := ShouldIgnoreFile(repoPath, tt.filePath)
			assert.NilError(t, err)
			assert.Equal(t, ignore, false)
		})
	}
}

// @description    Verifies that tracked hidden files remain eligible.
//
// Test_ShouldIgnoreFile_TrackedHiddenFile verifies that a hidden file present in the Git index is
// not filtered before or after its worktree file is deleted.
//
// @param           t   "test handle used to create and delete a tracked hidden file"
func Test_ShouldIgnoreFile_TrackedHiddenFile(t *testing.T) {
	repoPath := t.TempDir()
	repo, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	filePath := filepath.Join(repoPath, ".tracked")
	assert.NilError(t, os.WriteFile(filePath, []byte("tracked"), 0o600))
	worktree, err := repo.Worktree()
	assert.NilError(t, err)
	_, err = worktree.Add(".tracked")
	assert.NilError(t, err)

	ignore, err := ShouldIgnoreFile(repoPath, filePath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)

	assert.NilError(t, os.Remove(filePath))
	ignore, err = ShouldIgnoreFile(repoPath, filePath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)
}

// @description    Verifies tracked files bypass every ignore condition.
//
// Test_ShouldIgnoreFile_TrackedFileBypassesAllFilters verifies that files present in the Git index
// remain eligible even when they are empty or matched by a Git ignore rule.
//
// @param           t   "test handle used to stage tracked files and assert eligibility"
func Test_ShouldIgnoreFile_TrackedFileBypassesAllFilters(t *testing.T) {
	repoPath := t.TempDir()
	repo, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	worktree, err := repo.Worktree()
	assert.NilError(t, err)

	emptyPath := filepath.Join(repoPath, "empty-tracked")
	assert.NilError(t, os.WriteFile(emptyPath, nil, 0o600))
	_, err = worktree.Add("empty-tracked")
	assert.NilError(t, err)

	ignoredPath := filepath.Join(repoPath, "ignored.txt")
	assert.NilError(t, os.WriteFile(ignoredPath, []byte("tracked"), 0o600))
	_, err = worktree.Add("ignored.txt")
	assert.NilError(t, err)

	assert.NilError(t, os.WriteFile(filepath.Join(repoPath, ".gitignore"), []byte("ignored.txt\n"), 0o600))

	ignore, err := ShouldIgnoreFile(repoPath, emptyPath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)

	ignore, err = ShouldIgnoreFile(repoPath, ignoredPath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)
}

// @description    Verifies existing filters still apply to hidden exceptions.
//
// Test_ShouldIgnoreFile_HiddenExceptionsUseExistingFilters verifies that empty and Git-ignored files
// remain excluded even when their paths qualify for a hidden-path exception.
//
// @param           t   "test handle used to configure existing ignore conditions"
func Test_ShouldIgnoreFile_HiddenExceptionsUseExistingFilters(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath
	assert.NilError(t, os.WriteFile(filepath.Join(repoPath, ".gitignore"), []byte("*.txt\n*.example\n"), 0o600))
	assert.NilError(t, os.MkdirAll(filepath.Join(repoPath, ".github"), 0o700))
	assert.NilError(t, os.WriteFile(filepath.Join(repoPath, ".github", "empty.yml"), nil, 0o600))

	for _, filePath := range []string{
		filepath.Join(".github", "ignored.txt"),
		".env.example",
		filepath.Join(".github", "empty.yml"),
	} {
		ignore, err := ShouldIgnoreFile(repoPath, filePath)
		assert.NilError(t, err)
		assert.Equal(t, ignore, true)
	}
}

// @description    Verifies safe handling when the checked path is the repository root.
//
// Test_ShouldIgnoreFile_RepoRoot verifies that the repository root itself is ignored without panic.
//
// @param           t   "test handle used for fixture setup and assertion"
func Test_ShouldIgnoreFile_RepoRoot(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath

	ignore, err := ShouldIgnoreFile(repoPath, repoPath)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)
}

// @description    Verifies safe handling when no path is provided.
//
// Test_ShouldIgnoreFile_EmptyPath verifies that ShouldIgnoreFile returns an error instead of
// panicking when given an empty filePath.
//
// @param           t   "test handle used for fixture setup and assertion"
func Test_ShouldIgnoreFile_EmptyPath(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath

	_, err := ShouldIgnoreFile(repoPath, "")
	assert.ErrorContains(t, err, "path")
}
