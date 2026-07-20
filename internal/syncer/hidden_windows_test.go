//go:build windows

package syncer

import (
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"golang.org/x/sys/windows"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := windows.SetFileAttributes(ptr, attrs|windows.FILE_ATTRIBUTE_HIDDEN); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Untracked file inside a hidden directory.
	hiddenDir := filepath.Join(repoPath, "cache")
	if err := os.MkdirAll(hiddenDir, 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	hiddenFile := filepath.Join(hiddenDir, "data.txt")
	if err := os.WriteFile(hiddenFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	setHidden(t, hiddenDir)
	ignore, err := ShouldIgnoreFile(repoPath, hiddenFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != true {
		t.Fatalf("got %v, want %v", ignore, true)
	}

	// Untracked file that is itself hidden.
	selfHidden := filepath.Join(repoPath, "secret.txt")
	if err := os.WriteFile(selfHidden, []byte("x"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	setHidden(t, selfHidden)
	ignore, err = ShouldIgnoreFile(repoPath, selfHidden)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != true {
		t.Fatalf("got %v, want %v", ignore, true)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hiddenDir := filepath.Join(repoPath, "tracked-cache")
	if err := os.MkdirAll(hiddenDir, 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	trackedFile := filepath.Join(hiddenDir, "data.txt")
	if err := os.WriteFile(trackedFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = worktree.Add(filepath.ToSlash(filepath.Join("tracked-cache", "data.txt")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	setHidden(t, hiddenDir)

	ignore, err := ShouldIgnoreFile(repoPath, trackedFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != false {
		t.Fatalf("got %v, want %v", ignore, false)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if isHiddenByOS(repoPath, repoPath) != false {
		t.Fatalf("got %v, want %v", isHiddenByOS(repoPath, repoPath), false)
	}
}
