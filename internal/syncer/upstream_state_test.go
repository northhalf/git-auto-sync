package syncer

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/northhalf/git-auto-sync/internal/config"
)

// @description    Verifies the equal synchronization state.
//
// Test_ResolveUpstreamState_Equal verifies that resolveUpstreamState reports equal when HEAD matches
// the upstream tip.
//
// @param           t   "test handle used for fixture setup and state assertion"
func Test_ResolveUpstreamState_Equal(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_nothing", []string{"rebase_parent"})

	state, err := resolveUpstreamState(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != upstreamStateEqual {
		t.Fatalf("got %v, want %v", state, upstreamStateEqual)
	}
}

// @description    Verifies the local-ahead synchronization state.
//
// Test_ResolveUpstreamState_LocalAhead verifies that resolveUpstreamState reports local-ahead when
// only HEAD has commits beyond the upstream tip.
//
// @param           t   "test handle used for fixture setup and state assertion"
func Test_ResolveUpstreamState_LocalAhead(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_local_commits", []string{"rebase_parent"})

	state, err := resolveUpstreamState(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != upstreamStateLocalAhead {
		t.Fatalf("got %v, want %v", state, upstreamStateLocalAhead)
	}
}

// @description    Verifies the upstream-ahead synchronization state.
//
// Test_ResolveUpstreamState_UpstreamAhead verifies that resolveUpstreamState reports upstream-ahead
// when only the upstream has commits beyond HEAD.
//
// @param           t   "test handle used for fixture setup and state assertion"
func Test_ResolveUpstreamState_UpstreamAhead(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_remote_commits", []string{"rebase_parent"})

	state, err := resolveUpstreamState(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != upstreamStateUpstreamAhead {
		t.Fatalf("got %v, want %v", state, upstreamStateUpstreamAhead)
	}
}

// @description    Verifies the diverged synchronization state.
//
// Test_ResolveUpstreamState_Diverged verifies that resolveUpstreamState reports diverged when both
// HEAD and the upstream have commits beyond their merge-base.
//
// @param           t   "test handle used for fixture setup and state assertion"
func Test_ResolveUpstreamState_Diverged(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "rebase_both_commits", []string{"rebase_parent"})

	state, err := resolveUpstreamState(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != upstreamStateDiverged {
		t.Fatalf("got %v, want %v", state, upstreamStateDiverged)
	}
}

// @description    Verifies the no-upstream synchronization state.
//
// Test_ResolveUpstreamState_NoUpstream verifies that resolveUpstreamState reports none when the
// current branch has no configured upstream, using a freshly initialized repository with a single
// commit so HEAD resolves.
//
// @param           t   "test handle used to create the repository and assert the state"
func Test_ResolveUpstreamState_NoUpstream(t *testing.T) {
	repoPath := t.TempDir()
	if err := exec.Command("git", "init", repoPath).Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := exec.Command("git", "-C", repoPath, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := exec.Command("git", "-C", repoPath, "config", "user.name", "Git Auto Sync Tests").Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("initial"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := exec.Command("git", "-C", repoPath, "add", "file.txt").Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := exec.Command("git", "-C", repoPath, "commit", "-m", "initial").Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.NewRepoConfig(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state, err := resolveUpstreamState(slog.Default(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != upstreamStateNone {
		t.Fatalf("got %v, want %v", state, upstreamStateNone)
	}
}
