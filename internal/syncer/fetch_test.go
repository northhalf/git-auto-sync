package syncer

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/northhalf/git-auto-sync/internal/config"
)

// @description    Prepares related repository fixtures.
//
// PrepareMultiFixtures creates a temporary testdata directory, copies each dependency fixture into
// it, renames each .gitted directory to .git, prepares the named repository fixture, rewrites its
// remote paths to the copied dependencies, and returns its configuration.
//
// @param           t      "test handle used for temporary directories and fixture assertions"
//
// @param           name   "name of the repository fixture to prepare"
//
// @param           deps   "names of dependency fixtures to copy into temporary testdata"
//
// @return          config.RepoConfig   "configuration for the prepared repository fixture"
func PrepareMultiFixtures(t *testing.T, name string, deps []string) config.RepoConfig {
	newTestDataPath := t.TempDir()

	for _, name := range deps {
		fixturePath := filepath.Join("testdata", name)
		newPath := filepath.Join(newTestDataPath, name)
		err := os.CopyFS(newPath, os.DirFS(fixturePath))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		err = os.Rename(filepath.Join(newPath, ".gitted"), filepath.Join(newPath, ".git"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	repoConfig := PrepareFixture(t, name)
	FixFixtureGitConfig(t, repoConfig.RepoPath, newTestDataPath)

	return repoConfig
}

// @description    Rewrites fixture remote paths.
//
// FixFixtureGitConfig rewrites every $TESTDATA$ placeholder in a fixture repository's Git config
// to the temporary testdata path so its remotes use the copied fixtures.
//
// @param           t              "test handle used for file-operation assertions"
//
// @param           newRepoPath    "path to the prepared repository fixture"
//
// @param           testDataPath   "path to the temporary directory containing dependency fixtures"
func FixFixtureGitConfig(t *testing.T, newRepoPath string, testDataPath string) {
	dotGitPath := filepath.Join(newRepoPath, ".git")
	gitConfigFilePath := filepath.Join(dotGitPath, "config")
	input, err := os.ReadFile(gitConfigFilePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := bytes.ReplaceAll(input, []byte("$TESTDATA$"), []byte(testDataPath))

	err = os.WriteFile(gitConfigFilePath, output, 0666)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// @description    Configures the fixture branch to track an upstream branch.
//
// setFixtureUpstream sets branch.master.remote and branch.master.merge in the prepared fixture so
// its master branch tracks upstreamBranch on upstreamRemote, mirroring the upstream configuration
// that a cloned repository carries.
//
// @param           t               "test handle used for command assertions"
//
// @param           repoPath        "path to the prepared repository fixture"
//
// @param           upstreamRemote  "remote name the master branch tracks"
//
// @param           upstreamBranch  "branch on the remote the master branch tracks"
func setFixtureUpstream(t *testing.T, repoPath string, upstreamRemote string, upstreamBranch string) {
	t.Helper()
	if err := exec.Command("git", "-C", repoPath, "config", "branch.master.remote", upstreamRemote).Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := exec.Command("git", "-C", repoPath, "config", "branch.master.merge", "refs/heads/"+upstreamBranch).Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// @description    Verifies remote-tracking updates.
//
// Test_SimpleFetch verifies that fetching leaves the local HEAD unchanged while updating the
// origin1/master remote-tracking reference to the dependency fixture's commit. simple_fetch ships
// without a tracking branch, so the test configures one for fetch to resolve.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_SimpleFetch(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})
	setFixtureUpstream(t, repoConfig.RepoPath, "origin1", "master")

	err := fetch(slog.Default(), repoConfig)
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

	ref, err := r.Reference(plumbing.NewRemoteReferenceName("origin1", "master"), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ref.Hash() != plumbing.NewHash("7058b6b292ee3d1382670334b5f29570a1117ef1") {
		t.Fatalf("got %v, want %v", ref.Hash(), plumbing.NewHash("7058b6b292ee3d1382670334b5f29570a1117ef1"))
	}
}

// @description    Verifies fetching skips a branch without an upstream.
//
// Test_FetchSkipsWithoutUpstream verifies that fetch succeeds without contacting the configured
// remote when the current branch has no upstream tracking branch: simple_fetch ships without one,
// so its origin1/master remote-tracking reference must remain absent.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_FetchSkipsWithoutUpstream(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})

	err := fetch(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, err := git.PlainOpen(repoConfig.RepoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = r.Reference(plumbing.NewRemoteReferenceName("origin1", "master"), true)
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		t.Fatalf("error %v is not %v", err, plumbing.ErrReferenceNotFound)
	}
}

// @description    Verifies fetch only updates the upstream remote.
//
// Test_FetchOnlyUpstreamRemote verifies that fetch updates the current branch's upstream
// remote-tracking reference while leaving a second configured remote pointing at the same
// repository unfetched.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_FetchOnlyUpstreamRemote(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})
	setFixtureUpstream(t, repoConfig.RepoPath, "origin1", "master")

	remoteURL, err := exec.Command("git", "-C", repoConfig.RepoPath, "remote", "get-url", "origin1").Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := exec.Command("git", "-C", repoConfig.RepoPath, "remote", "add", "origin2", strings.TrimSpace(string(remoteURL))).Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = fetch(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, err := git.PlainOpen(repoConfig.RepoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ref, err := r.Reference(plumbing.NewRemoteReferenceName("origin1", "master"), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Hash() != plumbing.NewHash("7058b6b292ee3d1382670334b5f29570a1117ef1") {
		t.Fatalf("got %v, want %v", ref.Hash(), plumbing.NewHash("7058b6b292ee3d1382670334b5f29570a1117ef1"))
	}

	_, err = r.Reference(plumbing.NewRemoteReferenceName("origin2", "master"), true)
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		t.Fatalf("error %v is not %v", err, plumbing.ErrReferenceNotFound)
	}
}

// @description    Verifies fetching skips a branch tracking a local branch.
//
// Test_FetchSkipsLocalUpstream verifies that fetch succeeds without a network operation when the
// current branch tracks another local branch, recorded as upstream remote ".".
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_FetchSkipsLocalUpstream(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})
	setFixtureUpstream(t, repoConfig.RepoPath, ".", "master")

	err := fetch(slog.Default(), repoConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// @description    Verifies an unreachable upstream remote fails the fetch.
//
// Test_FetchRemoteFailure verifies that fetch returns the Git error when the upstream remote URL
// does not name a repository.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_FetchRemoteFailure(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})
	setFixtureUpstream(t, repoConfig.RepoPath, "origin1", "master")

	missingRemote := filepath.Join(t.TempDir(), "missing.git")
	if err := exec.Command("git", "-C", repoConfig.RepoPath, "remote", "set-url", "origin1", missingRemote).Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := fetch(slog.Default(), repoConfig)
	if err == nil {
		t.Fatalf("assertion failed: err != nil")
	}
}
