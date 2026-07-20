package syncer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, p := range paths {
		got, err := checker.ShouldIgnore(p)
		want, wantErr := ShouldIgnoreFile(repoPath, p)

		if p == "" {
			if err == nil || !strings.Contains(err.Error(), "path") {
				t.Fatalf("error %v does not contain %q", err, "path")
			}
			if wantErr == nil || !strings.Contains(wantErr.Error(), "path") {
				t.Fatalf("error %v does not contain %q", wantErr, "path")
			}
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if wantErr != nil {
			t.Fatalf("unexpected error: %v", wantErr)
		}
		if got != want {
			t.Fatalf("mismatch for path %q: got %v, want %v", p, got, want)
		}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A tracked file (staged) must bypass every filter.
	trackedPath := filepath.Join(repoPath, ".tracked")
	if err := os.WriteFile(trackedPath, []byte("tracked"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = worktree.Add(".tracked")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// An untracked hidden file must be ignored.
	hiddenPath := filepath.Join(repoPath, ".cache", "file")
	if err := os.MkdirAll(filepath.Dir(hiddenPath), 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(hiddenPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// An untracked non-hidden, non-ignored file must be eligible.
	plainPath := filepath.Join(repoPath, "plain.md")
	if err := os.WriteFile(plainPath, []byte("plain"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checker, err := NewIgnoreChecker(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ignore, err := checker.ShouldIgnore(trackedPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != false {
		t.Fatalf("got %v, want %v", ignore, false)
	}

	ignore, err = checker.ShouldIgnore(hiddenPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != true {
		t.Fatalf("got %v, want %v", ignore, true)
	}

	ignore, err = checker.ShouldIgnore(plainPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != false {
		t.Fatalf("got %v, want %v", ignore, false)
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trackedIgnoredPath := filepath.Join(repoPath, "ignored.txt")
	if err := os.WriteFile(trackedIgnoredPath, []byte("tracked"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = worktree.Add("ignored.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	untrackedIgnoredPath := filepath.Join(repoPath, "untracked.txt")
	if err := os.WriteFile(untrackedIgnoredPath, []byte("untracked"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(repoPath, ".gitignore"), []byte("*.txt\n"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checker, err := NewIgnoreChecker(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ignore, err := checker.ShouldIgnore(trackedIgnoredPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != false {
		t.Fatalf("got %v, want %v", ignore, false)
	}

	ignore, err = checker.ShouldIgnore(untrackedIgnoredPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != true {
		t.Fatalf("got %v, want %v", ignore, true)
	}
}

// @description    Verifies events through a resolved repository path stay inside the repository.
//
// Test_IgnoreChecker_ResolvesRepositorySymlink opens a repository through a directory symlink and
// checks a Git lock event reported through the symlink target. The event must resolve to
// .git/index.lock and be ignored rather than rejected as outside the repository.
//
// @param           t   "test handle used to create the repository symlink and assert"
func Test_IgnoreChecker_ResolvesRepositorySymlink(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	if err := os.Mkdir(repoPath, 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	linkedPath := filepath.Join(root, "linked-repo")
	if err := os.Symlink(repoPath, linkedPath); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}

	checker, err := NewIgnoreChecker(linkedPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ignore, err := checker.ShouldIgnore(filepath.Join(repoPath, ".git", "index.lock"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != true {
		t.Fatalf("got %v, want %v", ignore, true)
	}
}

// @description    Verifies events reported through a symlink resolve against the real repository.
//
// Test_IgnoreChecker_ResolvesEventSymlink opens a repository through its real path and checks a
// missing Git lock event reported through a directory symlink. Resolving the nearest existing parent
// must classify the event as .git/index.lock even though the lock file no longer exists.
//
// @param           t   "test handle used to create the repository symlink and assert"
func Test_IgnoreChecker_ResolvesEventSymlink(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	if err := os.Mkdir(repoPath, 0o700); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	linkedPath := filepath.Join(root, "linked-repo")
	if err := os.Symlink(repoPath, linkedPath); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}

	checker, err := NewIgnoreChecker(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ignore, err := checker.ShouldIgnore(filepath.Join(linkedPath, ".git", "index.lock"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ignore != true {
		t.Fatalf("got %v, want %v", ignore, true)
	}
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
	if err == nil {
		t.Fatalf("assertion failed: err != nil")
	}
}
