package config

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
)

// @description    Writes global settings into an isolated config directory for the test.
//
// withGlobal points XDG_CONFIG_HOME and HOME at a fresh temp dir and persists s, so NewRepoConfig
// reads it as the global configuration.
//
// @param           t   "test handle used to isolate the configuration directory"
//
// @param           s   "global settings to persist"
func withGlobal(t *testing.T, s *Settings) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if err := WriteGlobalSettings(s); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// @description    Verifies the default debounce duration.
//
// Test_NewRepoConfigDefaultDebounce verifies that repositories without explicit auto-sync timing
// use the ten-minute default debounce.
//
// @param           t   "test handle used to create the repository and assert its configuration"
func Test_NewRepoConfigDefaultDebounce(t *testing.T) {
	withGlobal(t, &Settings{})
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := NewRepoConfig(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Debounce != 10*time.Minute {
		t.Fatalf("got %v, want %v", cfg.Debounce, 10*time.Minute)
	}
	if cfg.SyncInterval != 60*time.Minute {
		t.Fatalf("got %v, want %v", cfg.SyncInterval, 60*time.Minute)
	}
	if cfg.GitExec != "git" {
		t.Fatalf("got %v, want %v", cfg.GitExec, "git")
	}
}

// @description    Verifies global settings apply when local is unset.
//
// Test_NewRepoConfigGlobal verifies that a global syncInterval applies to a repository with no
// local override.
//
// @param           t   "test handle used to create the repository and assert its configuration"
func Test_NewRepoConfigGlobal(t *testing.T) {
	sixty := 120
	withGlobal(t, &Settings{SyncInterval: &sixty})
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := NewRepoConfig(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SyncInterval != 120*time.Minute {
		t.Fatalf("got %v, want %v", cfg.SyncInterval, 120*time.Minute)
	}
}

// @description    Verifies local settings override global.
//
// Test_NewRepoConfigLocalOverridesGlobal verifies that a local syncInterval overrides the global
// value.
//
// @param           t   "test handle used to create the repository and assert its configuration"
func Test_NewRepoConfigLocalOverridesGlobal(t *testing.T) {
	g := 120
	withGlobal(t, &Settings{SyncInterval: &g})
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := SetLocalSetting(repoPath, "syncInterval", "30"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := NewRepoConfig(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SyncInterval != 30*time.Minute {
		t.Fatalf("got %v, want %v", cfg.SyncInterval, 30*time.Minute)
	}
}

// @description    Verifies an explicit nonexistent gitexec errors.
//
// Test_NewRepoConfigGitExecMissing verifies that an explicitly configured gitexec that does not
// exist returns an error.
//
// @param           t   "test handle used to create the repository and assert the error"
func Test_NewRepoConfigGitExecMissing(t *testing.T) {
	withGlobal(t, &Settings{})
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := SetLocalSetting(repoPath, "gitexec", "/does/not/exist/git"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = NewRepoConfig(repoPath)
	if err == nil {
		t.Fatalf("assertion failed: err != nil")
	}
}
