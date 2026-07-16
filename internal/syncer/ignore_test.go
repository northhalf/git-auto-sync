package syncer

import (
	"path/filepath"
	"testing"

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

// @description    Verifies handling of hidden files.
//
// Test_HiddenFilesIgnore verifies that ShouldIgnoreFile returns false for the nonexistent .hidden
// path because its dot prefix and the fixture's *.txt ignore rule do not ignore it.
//
// @param           t   "test handle used for fixture setup and ignore assertions"
func Test_HiddenFilesIgnore(t *testing.T) {
	repoPath := PrepareFixture(t, "ignore").RepoPath

	ignore, err := ShouldIgnoreFile(repoPath, ".hidden")
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)
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
