package config

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"gotest.tools/v3/assert"
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
	assert.NilError(t, WriteGlobalSettings(s))
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
	assert.NilError(t, err)

	cfg, err := NewRepoConfig(repoPath)
	assert.NilError(t, err)
	assert.Equal(t, cfg.Debounce, 10*time.Minute)
	assert.Equal(t, cfg.SyncInterval, 60*time.Minute)
	assert.Equal(t, cfg.GitExec, "git")
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
	assert.NilError(t, err)

	cfg, err := NewRepoConfig(repoPath)
	assert.NilError(t, err)
	assert.Equal(t, cfg.SyncInterval, 120*time.Minute)
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
	assert.NilError(t, err)
	assert.NilError(t, SetLocalSetting(repoPath, "syncInterval", "30"))

	cfg, err := NewRepoConfig(repoPath)
	assert.NilError(t, err)
	assert.Equal(t, cfg.SyncInterval, 30*time.Minute)
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
	assert.NilError(t, err)
	assert.NilError(t, SetLocalSetting(repoPath, "gitexec", "/does/not/exist/git"))

	_, err = NewRepoConfig(repoPath)
	assert.Assert(t, err != nil)
}
