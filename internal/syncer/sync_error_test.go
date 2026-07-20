package syncer

import (
	"bytes"
	"errors"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/northhalf/git-auto-sync/internal/notification"
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
		{stage: "compare"},
		{stage: "rebase"},
		{stage: "push", remote: true},
	}

	for _, tt := range tests {
		t.Run(tt.stage, func(t *testing.T) {
			cause := errors.New("stage failed")
			err := newSyncError(tt.stage, cause)

			if SyncErrorStage(err) != tt.stage {
				t.Fatalf("got %v, want %v", SyncErrorStage(err), tt.stage)
			}
			if IsRemoteSyncError(err) != tt.remote {
				t.Fatalf("got %v, want %v", IsRemoteSyncError(err), tt.remote)
			}
			if !errors.Is(err, cause) {
				t.Fatalf("assertion failed: errors.Is(err, cause)")
			}
		})
	}
}

// @description    Verifies author-stage classification from AutoSync.
//
// Test_AutoSyncClassifiesAuthorFailure verifies that AutoSync labels a missing Git identity as an
// author-stage error while preserving the original missing-email error. It uses the rebase_nothing
// fixture, which has a commit and a configured upstream, so the repository passes the repo-state
// precheck; isolating HOME removes any global Git identity so AutoSync fails at the author stage.
//
// @param           t   "test handle used to create the repository and assert the returned error"
func Test_AutoSyncClassifiesAuthorFailure(t *testing.T) {
	cfg := PrepareMultiFixtures(t, "rebase_nothing", []string{"rebase_parent"})

	home := t.TempDir()
	cfg.Env = []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + home,
		"GIT_CONFIG_GLOBAL=" + filepath.Join(home, "global.gitconfig"),
	}

	err := AutoSync(slog.Default(), cfg)
	if SyncErrorStage(err) != SyncStageAuthor {
		t.Fatalf("got %v, want %v", SyncErrorStage(err), SyncStageAuthor)
	}
	if !errors.Is(err, errNoGitAuthorEmail) {
		t.Fatalf("assertion failed: errors.Is(err, errNoGitAuthorEmail)")
	}
}

// @description    Verifies fetch-stage classification from AutoSync.
//
// Test_AutoSyncClassifiesFetchFailure verifies that AutoSync labels a failed remote fetch as a
// retryable fetch-stage error. simple_fetch ships without a tracking branch, so the test configures
// one so the repository passes the repo-state precheck and AutoSync reaches the fetch stage before
// the rewritten remote URL fails.
//
// @param           t   "test handle used to prepare the fixture and assert the returned error"
func Test_AutoSyncClassifiesFetchFailure(t *testing.T) {
	cfg := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})
	if err := exec.Command("git", "-C", cfg.RepoPath, "config", "branch.master.remote", "origin1").Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := exec.Command("git", "-C", cfg.RepoPath, "config", "branch.master.merge", "refs/heads/master").Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	missingRemote := filepath.Join(t.TempDir(), "missing.git")
	cmd := exec.Command("git", "-C", cfg.RepoPath, "remote", "set-url", "origin1", missingRemote)
	if err := cmd.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := AutoSync(slog.Default(), cfg)
	if SyncErrorStage(err) != SyncStageFetch {
		t.Fatalf("got %v, want %v", SyncErrorStage(err), SyncStageFetch)
	}
	if !IsRemoteSyncError(err) {
		t.Fatalf("assertion failed: IsRemoteSyncError(err)")
	}
}

// @description    Verifies a repo-state pause reports its real stage when the alert cannot fire.
//
// Test_AutoSync_RepoStatePauseStage verifies that AutoSync returns the specific repo-state stage
// when checkRepoState blocks the repository, rather than masking it as an alert-stage error. This
// matters on headless hosts where beeep.Alert cannot deliver a desktop notification: the real
// reason must still reach state.json so daemon status shows why the repository paused.
//
// @param           t   "test handle used to prepare the fixture and assert the returned stage"
func Test_AutoSync_RepoStatePauseStage(t *testing.T) {
	// simple_fetch has no tracking branch, so checkRepoState rejects it before any mutation.
	cfg := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})

	err := AutoSync(slog.Default(), cfg)
	if SyncErrorStage(err) != "no-upstream" {
		t.Fatalf("got %v, want %v", SyncErrorStage(err), "no-upstream")
	}
	if !errors.Is(err, errNoUpstream) {
		t.Fatalf("assertion failed: errors.Is(err, errNoUpstream)")
	}
}

// @description    Verifies an already-reported unavailable notifier does not add duplicate sync warnings.
//
// @param           t  "test handle used to prepare a paused repository and inspect logs"
func Test_AutoSyncUnavailableAlertDoesNotRepeatWarning(t *testing.T) {
	previous := sendAlert
	sendAlert = func(string, string) error { return notification.ErrUnavailable }
	t.Cleanup(func() { sendAlert = previous })

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cfg := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})

	err := AutoSync(logger, cfg)
	if SyncErrorStage(err) != "no-upstream" {
		t.Fatalf("got %v, want %v", SyncErrorStage(err), "no-upstream")
	}
	if !errors.Is(err, errNoUpstream) {
		t.Fatalf("assertion failed: errors.Is(err, errNoUpstream)")
	}
	if strings.Contains(logs.String(), "send repo state alert failed") {
		t.Fatalf("assertion failed: !strings.Contains(logs.String(), \"send repo state alert failed\")")
	}
}
