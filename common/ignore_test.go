package common

import (
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
