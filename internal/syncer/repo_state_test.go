package syncer

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/northhalf/git-auto-sync/internal/config"
	"gotest.tools/v3/assert"
)

// @description    Creates a fresh repository with one commit.
//
// initRepoWithCommit initializes a repository, configures a test Git identity, and makes a single
// commit so HEAD resolves to a branch. It mirrors Test_ResolveUpstreamState_NoUpstream's setup for
// tests that need a repository without an upstream tracking branch.
//
// @param           t     "test handle used for temporary directory and Git command assertions"
//
// @return          string "absolute path to the initialized repository"
func initRepoWithCommit(t *testing.T) string {
	t.Helper()
	repoPath := t.TempDir()
	assert.NilError(t, exec.Command("git", "init", repoPath).Run())
	assert.NilError(t, exec.Command("git", "-C", repoPath, "config", "user.email", "test@example.com").Run())
	assert.NilError(t, exec.Command("git", "-C", repoPath, "config", "user.name", "Git Auto Sync Tests").Run())
	assert.NilError(t, os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("initial"), 0o644))
	assert.NilError(t, exec.Command("git", "-C", repoPath, "add", "file.txt").Run())
	assert.NilError(t, exec.Command("git", "-C", repoPath, "commit", "-m", "initial").Run())
	return repoPath
}

// @description    Verifies that a syncable repository passes the state check.
//
// Test_CheckRepoState_HappyPath verifies that checkRepoState returns nil for a repository whose
// current branch has a configured upstream and no operation in progress.
//
// @param           t   "test handle used for fixture setup and assertion"
func Test_CheckRepoState_HappyPath(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_nothing", []string{"rebase_parent"})

	err := checkRepoState(slog.Default(), repoConfig)
	assert.NilError(t, err)
}

// @description    Verifies that a branch without an upstream is rejected.
//
// Test_CheckRepoState_NoUpstream verifies that checkRepoState returns errNoUpstream for a freshly
// initialized repository whose branch has no configured tracking branch.
//
// @param           t   "test handle used for repository setup and assertion"
func Test_CheckRepoState_NoUpstream(t *testing.T) {
	repoPath := initRepoWithCommit(t)
	cfg, err := config.NewRepoConfig(repoPath)
	assert.NilError(t, err)

	err = checkRepoState(slog.Default(), cfg)
	assert.Assert(t, err != nil, "expected an error for a branch without upstream")
	assert.Assert(t, errors.Is(err, errNoUpstream), "expected errNoUpstream, got %v", err)
}

// @description    Verifies that a detached HEAD is rejected before the upstream check.
//
// Test_CheckRepoState_DetachedHead verifies that checkRepoState returns errDetachedHead when HEAD is
// detached, and that the detached-HEAD check runs before the upstream check so the more specific
// reason is reported.
//
// @param           t   "test handle used for repository setup and assertion"
func Test_CheckRepoState_DetachedHead(t *testing.T) {
	repoPath := initRepoWithCommit(t)
	assert.NilError(t, exec.Command("git", "-C", repoPath, "checkout", "--detach").Run())
	cfg, err := config.NewRepoConfig(repoPath)
	assert.NilError(t, err)

	err = checkRepoState(slog.Default(), cfg)
	assert.Assert(t, err != nil, "expected an error for a detached HEAD")
	assert.Assert(t, errors.Is(err, errDetachedHead), "expected errDetachedHead, got %v", err)
}

// @description    Verifies that an in-progress Git operation is rejected.
//
// Test_CheckRepoState_Busy verifies that checkRepoState returns errRepoBusy when the repository's
// .git directory contains any of the markers Git writes during a merge, rebase, cherry-pick, or
// revert. Each subtest uses a fresh fixture so markers do not carry over.
//
// @param           t   "test handle used for fixture setup and per-marker assertions"
func Test_CheckRepoState_Busy(t *testing.T) {
	tests := []struct {
		name   string
		marker string
		isDir  bool
	}{
		{"merge", "MERGE_HEAD", false},
		{"rebase-merge", "rebase-merge", true},
		{"rebase-apply", "rebase-apply", true},
		{"cherry-pick", "CHERRY_PICK_HEAD", false},
		{"revert", "REVERT_HEAD", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoConfig := PrepareMultiFixtures(t, "rebase_nothing", []string{"rebase_parent"})
			markerPath := filepath.Join(repoConfig.RepoPath, ".git", tt.marker)
			if tt.isDir {
				assert.NilError(t, os.Mkdir(markerPath, 0o755))
			} else {
				assert.NilError(t, os.WriteFile(markerPath, []byte("deadbeef\n"), 0o644))
			}

			err := checkRepoState(slog.Default(), repoConfig)
			assert.Assert(t, err != nil, "expected an error for marker %s", tt.marker)
			assert.Assert(t, errors.Is(err, errRepoBusy), "expected errRepoBusy for %s, got %v", tt.name, err)
		})
	}
}

// @description    Verifies the repo-state error to stage mapping.
//
// Test_RepoStateStage verifies that repoStateStage maps each checkRepoState sentinel error to its
// daemon-state stage label and falls back to the generic repo-state stage for any other error.
//
// @param           t   "test handle used for table-driven assertions"
func Test_RepoStateStage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"busy", fmt.Errorf("%w: merge", errRepoBusy), "repo-busy"},
		{"detached", errDetachedHead, "detached-head"},
		{"no-upstream", errNoUpstream, "no-upstream"},
		{"other", errors.New("boom"), syncStageRepoState},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, repoStateStage(tt.err), tt.want)
		})
	}
}
