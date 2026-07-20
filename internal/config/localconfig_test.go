package config

import (
	"testing"

	git "github.com/go-git/go-git/v5"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := SetLocalSetting(repoPath, "syncInterval", "30"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := SetLocalSetting(repoPath, "debounce", "5"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := SetLocalSetting(repoPath, "gitexec", "/usr/bin/git"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	local, err := ReadLocalSettings(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if local.Repos != nil {
		t.Fatalf("assertion failed: local.Repos == nil")
	}
	if local.Envs != nil {
		t.Fatalf("assertion failed: local.Envs == nil")
	}
	if *local.SyncInterval != 30 {
		t.Fatalf("got %v, want %v", *local.SyncInterval, 30)
	}
	if *local.Debounce != 5 {
		t.Fatalf("got %v, want %v", *local.Debounce, 5)
	}
	if *local.GitExec != "/usr/bin/git" {
		t.Fatalf("got %v, want %v", *local.GitExec, "/usr/bin/git")
	}

	if err := UnsetLocalSetting(repoPath, "syncInterval"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := UnsetLocalSetting(repoPath, "debounce"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := UnsetLocalSetting(repoPath, "gitexec"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	local, err = ReadLocalSettings(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if local.SyncInterval != nil {
		t.Fatalf("assertion failed: local.SyncInterval == nil")
	}
	if local.Debounce != nil {
		t.Fatalf("assertion failed: local.Debounce == nil")
	}
	if local.GitExec != nil {
		t.Fatalf("assertion failed: local.GitExec == nil")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	local, err := ReadLocalSettings(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if local.SyncInterval != nil {
		t.Fatalf("assertion failed: local.SyncInterval == nil")
	}
	if local.Debounce != nil {
		t.Fatalf("assertion failed: local.Debounce == nil")
	}
	if local.GitExec != nil {
		t.Fatalf("assertion failed: local.GitExec == nil")
	}
}

// @description    Verifies SetLocalSetting replaces an existing value.
//
// Test_SetLocalSettingReplaces verifies that setting a key twice stores the latest value.
//
// @param           t   "test handle used to create the repository and assert settings"
func Test_SetLocalSettingReplaces(t *testing.T) {
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := SetLocalSetting(repoPath, "syncInterval", "30"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := SetLocalSetting(repoPath, "syncInterval", "45"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	local, err := ReadLocalSettings(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *local.SyncInterval != 45 {
		t.Fatalf("got %v, want %v", *local.SyncInterval, 45)
	}
}
