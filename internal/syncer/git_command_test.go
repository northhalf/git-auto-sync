package syncer

import (
	"slices"

	"github.com/northhalf/git-auto-sync/internal/config"
	"testing"
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

// @description    Verifies environment keys whose values contain equals signs.
//
// @param           t  "test handle used for assertions"
func TestHasEnvVariableHandlesEqualsInValue(t *testing.T) {
	if !hasEnvVariable([]string{"COMPLEX=a=b"}, "COMPLEX") {
		t.Fatal("expected COMPLEX to be detected")
	}
}
