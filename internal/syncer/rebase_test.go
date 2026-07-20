package syncer

import (
	"log/slog"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// @description    Verifies a rebase with no new commits.
//
// Test_RebaseNothing verifies that rebasing with no new remote or local commits succeeds and
// leaves HEAD at the shared commit.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_RebaseNothing(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_nothing", []string{"rebase_parent"})

	err := rebase(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, err := git.PlainOpen(repoConfig.RepoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	head, err := r.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if head.Hash() != plumbing.NewHash("28cc969d97ddb7640f5e1428bbc8f2947d1ffd57") {
		t.Fatalf("got %v, want %v", head.Hash(), plumbing.NewHash("28cc969d97ddb7640f5e1428bbc8f2947d1ffd57"))
	}
}

// @description    Verifies a rebase with local commits.
//
// Test_RebaseLocalCommits verifies that rebasing when only the local branch has new commits
// succeeds and preserves the expected local HEAD commit.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_RebaseLocalCommits(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_local_commits", []string{"rebase_parent"})

	err := rebase(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, err := git.PlainOpen(repoConfig.RepoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	head, err := r.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if head.Hash() != plumbing.NewHash("7fc438e0c9cc4f58178a1efe8521e52f0f8ee688") {
		t.Fatalf("got %v, want %v", head.Hash(), plumbing.NewHash("7fc438e0c9cc4f58178a1efe8521e52f0f8ee688"))
	}
}

// @description    Verifies a rebase with remote commits.
//
// Test_RebaseRemoteCommits verifies that rebasing when only the remote branch has new commits
// succeeds and advances HEAD to the expected remote commit.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_RebaseRemoteCommits(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_remote_commits", []string{"rebase_parent"})

	err := rebase(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, err := git.PlainOpen(repoConfig.RepoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	head, err := r.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if head.Hash() != plumbing.NewHash("ccda8f2e691aa416791a10afc74ccdbd1cb419fe") {
		t.Fatalf("got %v, want %v", head.Hash(), plumbing.NewHash("ccda8f2e691aa416791a10afc74ccdbd1cb419fe"))
	}
}

// @description    Verifies a nonconflicting divergent rebase.
//
// Test_RebaseBothCommitsNoConflict verifies that rebasing new remote and local commits without a
// conflict succeeds and produces a rewritten HEAD distinct from both sides' commits.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_RebaseBothCommitsNoConflict(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_both_commits", []string{"rebase_parent"})

	err := rebase(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, err := git.PlainOpen(repoConfig.RepoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	head, err := r.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if head.Hash() == plumbing.NewHash("ccda8f2e691aa416791a10afc74ccdbd1cb419fe") {
		t.Errorf("assertion failed: head.Hash() != plumbing.NewHash(\"ccda8f2e691aa416791a10afc74ccdbd1cb419fe\")")
	}
	if head.Hash() == plumbing.NewHash("5779561afa9d074ae8d20974861c54757429aca9") {
		t.Errorf("assertion failed: head.Hash() != plumbing.NewHash(\"5779561afa9d074ae8d20974861c54757429aca9\")")
	}
	if head.Hash() == plumbing.NewHash("7fc438e0c9cc4f58178a1efe8521e52f0f8ee688") {
		t.Errorf("assertion failed: head.Hash() != plumbing.NewHash(\"7fc438e0c9cc4f58178a1efe8521e52f0f8ee688\")")
	}
}

// @description    Verifies a conflicting divergent rebase.
//
// Test_RebaseBothCommitsConflict verifies that conflicting new remote and local commits return
// errRebaseFailed and leave HEAD at its pre-rebase commit after the abort.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_RebaseBothCommitsConflict(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_both_commits_conflict", []string{"rebase_parent"})

	r, err := git.PlainOpen(repoConfig.RepoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	origHead, err := r.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = rebase(slog.Default(), repoConfig)
	if err != errRebaseFailed {
		t.Fatalf("got %v, want %v", err, errRebaseFailed)
	}

	newHead, err := r.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newHead.Hash() != origHead.Hash() {
		t.Errorf("assertion failed: newHead.Hash() == origHead.Hash()")
	}
}
