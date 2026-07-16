package syncer

import (
	"errors"
	"log/slog"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
	"gotest.tools/v3/assert"
)

// @description    Verifies synchronization error classification.
//
// Test_SyncErrorClassification verifies that stage information survives wrapping and that only
// fetch and push errors are classified as remote synchronization errors.
//
// @param           t   "test handle used for classification assertions"
func Test_SyncErrorClassification(t *testing.T) {
	tests := []struct {
		stage  string
		remote bool
	}{
		{stage: "author"},
		{stage: "commit"},
		{stage: "fetch", remote: true},
		{stage: "rebase"},
		{stage: "alert"},
		{stage: "push", remote: true},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			cause := errors.New("stage failed")
			err := tracerr.Wrap(newSyncError(tt.stage, cause))

			assert.Equal(t, SyncErrorStage(err), tt.stage)
			assert.Equal(t, IsRemoteSyncError(err), tt.remote)
			assert.Assert(t, errors.Is(err, cause))
		})
	}
}

// @description    Verifies author-stage classification from AutoSync.
//
// Test_AutoSyncClassifiesAuthorFailure verifies that AutoSync labels a missing Git identity as an
// author-stage error while preserving the original missing-email error.
//
// @param           t   "test handle used to create the repository and assert the returned error"
func Test_AutoSyncClassifiesAuthorFailure(t *testing.T) {
	repoPath := t.TempDir()
	cmd := exec.Command("git", "init", repoPath)
	assert.NilError(t, cmd.Run())

	home := t.TempDir()
	cfg := config.RepoConfig{
		RepoPath: repoPath,
		Env: []string{
			"HOME=" + home,
			"XDG_CONFIG_HOME=" + home,
			"GIT_CONFIG_GLOBAL=" + filepath.Join(home, "global.gitconfig"),
		},
	}

	err := AutoSync(slog.Default(), cfg)
	assert.Equal(t, SyncErrorStage(err), syncStageAuthor)
	assert.Assert(t, errors.Is(err, errNoGitAuthorEmail))
}

// @description    Verifies fetch-stage classification from AutoSync.
//
// Test_AutoSyncClassifiesFetchFailure verifies that AutoSync labels a failed remote fetch as a
// retryable fetch-stage error.
//
// @param           t   "test handle used to prepare the fixture and assert the returned error"
func Test_AutoSyncClassifiesFetchFailure(t *testing.T) {
	cfg := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})
	missingRemote := filepath.Join(t.TempDir(), "missing.git")
	cmd := exec.Command("git", "-C", cfg.RepoPath, "remote", "set-url", "origin1", missingRemote)
	assert.NilError(t, cmd.Run())

	err := AutoSync(slog.Default(), cfg)
	assert.Equal(t, SyncErrorStage(err), syncStageFetch)
	assert.Assert(t, IsRemoteSyncError(err))
}
