//go:build windows

package syncer

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"golang.org/x/sys/windows"
	"gotest.tools/v3/assert"
)

// @description    Sets the Windows hidden attribute on a path.
//
// setHidden marks path with FILE_ATTRIBUTE_HIDDEN while preserving its existing attributes.
//
// @param           t     "test handle used for assertion failures"
//
// @param           path  "filesystem path to mark hidden"
func setHidden(t *testing.T, path string) {
	t.Helper()
	ptr, err := windows.UTF16PtrFromString(path)
	assert.NilError(t, err)
	attrs, err := windows.GetFileAttributes(ptr)
	assert.NilError(t, err)
	assert.NilError(t, windows.SetFileAttributes(ptr, attrs|windows.FILE_ATTRIBUTE_HIDDEN))
}

// @description    Verifies Windows-hidden directories and files are ignored.
//
// Test_IsHiddenByOS_WindowsAttribute verifies that an untracked file inside a Windows-hidden
// directory and an untracked file that itself carries the hidden attribute are both ignored.
//
// @param           t   "test handle used for fixture setup and ignore assertions"
func Test_IsHiddenByOS_WindowsAttribute(t *testing.T) {
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	// Untracked file inside a hidden directory.
	hiddenDir := filepath.Join(repoPath, "cache")
	assert.NilError(t, os.MkdirAll(hiddenDir, 0o700))
	hiddenFile := filepath.Join(hiddenDir, "data.txt")
	assert.NilError(t, os.WriteFile(hiddenFile, []byte("x"), 0o600))
	setHidden(t, hiddenDir)
	ignore, err := ShouldIgnoreFile(repoPath, hiddenFile)
	assert.NilError(t, err)
	assert.Equal(t, ignore, true)

	// Untracked file that is itself hidden.
	selfHidden := filepath.Join(repoPath, "secret.txt")
	assert.NilError(t, os.WriteFile(selfHidden, []byte("x"), 0o600))
	setHidden(t, selfHidden)
	ignore, err = ShouldIgnoreFile(repoPath, selfHidden)
	assert.NilError(t, err)
	assert.Equal(t, ignore, true)
}

// @description    Verifies tracked files inside Windows-hidden directories remain eligible.
//
// Test_IsHiddenByOS_TrackedFileBypasses verifies that a tracked file inside a Windows-hidden
// directory is not ignored, because tracked files bypass every ignore check.
//
// @param           t   "test handle used to stage a tracked file and assert eligibility"
func Test_IsHiddenByOS_TrackedFileBypasses(t *testing.T) {
	repoPath := t.TempDir()
	repo, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	hiddenDir := filepath.Join(repoPath, "tracked-cache")
	assert.NilError(t, os.MkdirAll(hiddenDir, 0o700))
	trackedFile := filepath.Join(hiddenDir, "data.txt")
	assert.NilError(t, os.WriteFile(trackedFile, []byte("x"), 0o600))
	worktree, err := repo.Worktree()
	assert.NilError(t, err)
	_, err = worktree.Add(filepath.ToSlash(filepath.Join("tracked-cache", "data.txt")))
	assert.NilError(t, err)
	setHidden(t, hiddenDir)

	ignore, err := ShouldIgnoreFile(repoPath, trackedFile)
	assert.NilError(t, err)
	assert.Equal(t, ignore, false)
}

// @description    Verifies the repository root is never treated as hidden.
//
// Test_IsHiddenByOS_RepoRootNotHidden verifies that the repository root itself reports not hidden
// when queried directly, so the root is never ignored.
//
// @param           t   "test handle used for fixture setup and assertion"
func Test_IsHiddenByOS_RepoRootNotHidden(t *testing.T) {
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	assert.Equal(t, isHiddenByOS(repoPath, repoPath), false)
}
