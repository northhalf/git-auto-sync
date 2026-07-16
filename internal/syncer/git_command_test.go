package syncer

import (
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/northhalf/git-auto-sync/internal/config"
)

// @description    Verifies that configured Git environment entries are included once.
//
// @param           t  "test handle used for assertions"
func TestToEnvStringDoesNotDuplicateConfiguredEntries(t *testing.T) {
	t.Setenv("HOME", "/tmp/git-auto-sync-home")

	cfg := config.RepoConfig{Env: []string{"TOKEN=value", "COMPLEX=a=b"}}
	original := slices.Clone(cfg.Env)

	env := toEnvString(cfg)

	for _, entry := range original {
		count := 0
		for _, actual := range env {
			if actual == entry {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected %q once, got %d occurrences in %v", entry, count, env)
		}
	}

	if !slices.Equal(cfg.Env, original) {
		t.Fatalf("configured environment mutated: got %v, want %v", cfg.Env, original)
	}
}

// @description    Verifies that HOME is inherited by Git commands.
//
// @param           t  "test handle used for assertions"
func TestToEnvStringIncludesHome(t *testing.T) {
	t.Setenv("HOME", "/tmp/git-auto-sync-home")

	env := toEnvString(config.RepoConfig{})
	if !slices.Contains(env, "HOME=/tmp/git-auto-sync-home") {
		t.Fatalf("expected HOME in environment, got %v", env)
	}
}

// @description    Verifies that the parent environment is inherited.
//
// @param           t  "test handle used for assertions"
func TestToEnvStringInheritsParentEnvironment(t *testing.T) {
	t.Setenv("GIT_AUTO_SYNC_TEST_VAR", "inherited")

	env := toEnvString(config.RepoConfig{})
	if !slices.Contains(env, "GIT_AUTO_SYNC_TEST_VAR=inherited") {
		t.Fatalf("expected inherited parent variable, got %v", env)
	}
}

// @description    Verifies that configured entries override inherited values.
//
// @param           t  "test handle used for assertions"
func TestToEnvStringOverridesInheritedEntries(t *testing.T) {
	t.Setenv("GIT_AUTO_SYNC_TEST_VAR", "inherited")

	cfg := config.RepoConfig{Env: []string{"GIT_AUTO_SYNC_TEST_VAR=explicit"}}
	env := toEnvString(cfg)

	if !slices.Contains(env, "GIT_AUTO_SYNC_TEST_VAR=explicit") {
		t.Fatalf("expected configured override, got %v", env)
	}
	if slices.Contains(env, "GIT_AUTO_SYNC_TEST_VAR=inherited") {
		t.Fatalf("expected inherited value to be replaced, got %v", env)
	}
}

// @description    Verifies that command errors expose env keys but not values.
//
// TestGitCommandErrorOmitsEnvValues triggers a failing Git command with a secret in the
// repository environment and asserts the returned error names the variable key while never
// containing the secret value.
//
// @param           t  "test handle used for assertions"
func TestGitCommandErrorOmitsEnvValues(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	secret := "super-secret-token-value"
	cfg := config.RepoConfig{
		RepoPath: t.TempDir(),
		Env:      []string{"GIT_AUTO_SYNC_SECRET=" + secret},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	_, err := gitCommand(logger, cfg, []string{"not-a-real-git-subcommand"})
	if err == nil {
		t.Fatal("expected a git command error, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "GIT_AUTO_SYNC_SECRET") {
		t.Fatalf("expected env key in error, got: %s", msg)
	}
	if strings.Contains(msg, secret) {
		t.Fatalf("env value leaked into error: %s", msg)
	}
}
