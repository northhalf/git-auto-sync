package config

import (
	"testing"

	git "github.com/go-git/go-git/v5"
	"gotest.tools/v3/assert"
)

// @description    Verifies local settings set, read, and unset round trip.
//
// Test_LocalSettingsRoundTrip sets each of the three [auto-sync] keys in a temp repository, reads
// them back, unsets them, and confirms they read as nil.
//
// @param           t   "test handle used to create the repository and assert settings"
func Test_LocalSettingsRoundTrip(t *testing.T) {
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	assert.NilError(t, SetLocalSetting(repoPath, "syncInterval", "30"))
	assert.NilError(t, SetLocalSetting(repoPath, "debounce", "5"))
	assert.NilError(t, SetLocalSetting(repoPath, "gitexec", "/usr/bin/git"))

	local, err := ReadLocalSettings(repoPath)
	assert.NilError(t, err)
	assert.Assert(t, local.Repos == nil)
	assert.Assert(t, local.Envs == nil)
	assert.Equal(t, *local.SyncInterval, 30)
	assert.Equal(t, *local.Debounce, 5)
	assert.Equal(t, *local.GitExec, "/usr/bin/git")

	assert.NilError(t, UnsetLocalSetting(repoPath, "syncInterval"))
	assert.NilError(t, UnsetLocalSetting(repoPath, "debounce"))
	assert.NilError(t, UnsetLocalSetting(repoPath, "gitexec"))

	local, err = ReadLocalSettings(repoPath)
	assert.NilError(t, err)
	assert.Assert(t, local.SyncInterval == nil)
	assert.Assert(t, local.Debounce == nil)
	assert.Assert(t, local.GitExec == nil)
}

// @description    Verifies ReadLocalSettings returns nil fields for an unset section.
//
// Test_ReadLocalSettingsEmpty verifies that a fresh repository without an [auto-sync] section reads
// back a Settings with all three optional fields nil.
//
// @param           t   "test handle used to create the repository and assert settings"
func Test_ReadLocalSettingsEmpty(t *testing.T) {
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	local, err := ReadLocalSettings(repoPath)
	assert.NilError(t, err)
	assert.Assert(t, local.SyncInterval == nil)
	assert.Assert(t, local.Debounce == nil)
	assert.Assert(t, local.GitExec == nil)
}

// @description    Verifies SetLocalSetting replaces an existing value.
//
// Test_SetLocalSettingReplaces verifies that setting a key twice stores the latest value.
//
// @param           t   "test handle used to create the repository and assert settings"
func Test_SetLocalSettingReplaces(t *testing.T) {
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	assert.NilError(t, SetLocalSetting(repoPath, "syncInterval", "30"))
	assert.NilError(t, SetLocalSetting(repoPath, "syncInterval", "45"))

	local, err := ReadLocalSettings(repoPath)
	assert.NilError(t, err)
	assert.Equal(t, *local.SyncInterval, 45)
}
