package config

import (
	"reflect"
	"testing"
	"time"
)

// @description    Prepares an isolated configuration directory.
//
// setup creates a temporary configuration directory, points XDG_CONFIG_HOME and HOME at it for the
// test, so ReadGlobalSettings and WriteGlobalSettings target an isolated file.
//
// @param           t      "test handle used for the temporary directory and environment changes"
func setup(t *testing.T) {
	newConfigDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", newConfigDir)
	t.Setenv("HOME", newConfigDir)
}

// @description    Verifies settings round trips with the new optional fields absent.
//
// Test_SettingsRoundTrip writes a Settings with repository and environment entries to an isolated
// config directory and reads back an equal value. The new optional fields stay nil.
//
// @param           t   "test handle used for isolated configuration setup and assertions"
func Test_SettingsRoundTrip(t *testing.T) {
	setup(t)

	c := &Settings{
		Repos: []string{"/home/xyz/hello"},
		Envs:  []string{"SSH_AUTH_SOCK=/private/tmp/com.apple.launchd.74ZznY1v1F/Listeners"},
	}
	err := WriteGlobalSettings(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c2, err := ReadGlobalSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(c, c2) {
		t.Fatalf("got %#v, want %#v", c, c2)
	}
}

// @description    Verifies the default constants.
//
// Test_Defaults verifies the package default constants match the raised defaults: one-hour sync
// interval, ten-minute debounce, and git resolved through PATH.
//
// @param           t   "test handle used for assertions"
func Test_Defaults(t *testing.T) {
	if DefaultSyncInterval != 60*time.Minute {
		t.Fatalf("got %v, want %v", DefaultSyncInterval, 60*time.Minute)
	}
	if DefaultDebounce != 10*time.Minute {
		t.Fatalf("got %v, want %v", DefaultDebounce, 10*time.Minute)
	}
	if DefaultGitExec != "git" {
		t.Fatalf("got %v, want %v", DefaultGitExec, "git")
	}
}

// @description    Verifies reads without a configuration file.
//
// Test_ReadEmpty verifies that reading from an isolated configuration directory with no file
// succeeds and returns a configuration with no repositories and nil optional fields.
//
// @param           t   "test handle used for isolated configuration setup and assertions"
func Test_ReadEmpty(t *testing.T) {
	setup(t)

	c, err := ReadGlobalSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.Repos) != 0 {
		t.Fatalf("assertion failed: len(c.Repos) == 0")
	}
	if c.SyncInterval != nil {
		t.Fatalf("assertion failed: c.SyncInterval == nil")
	}
	if c.Debounce != nil {
		t.Fatalf("assertion failed: c.Debounce == nil")
	}
	if c.GitExec != nil {
		t.Fatalf("assertion failed: c.GitExec == nil")
	}
}

// @description    Verifies the global settings modification time.
//
// Test_GlobalSettingsModTime verifies that the modification time is the zero time when no
// configuration file exists and a non-zero time after settings are written.
//
// @param           t   "test handle used for isolated configuration setup and assertions"
func Test_GlobalSettingsModTime(t *testing.T) {
	setup(t)

	mod, err := GlobalSettingsModTime()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mod.IsZero() {
		t.Fatalf("assertion failed: mod.IsZero()")
	}

	err = WriteGlobalSettings(&Settings{Repos: []string{"/repo"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mod, err = GlobalSettingsModTime()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mod.IsZero() {
		t.Fatalf("assertion failed: !mod.IsZero()")
	}
}

// @description    Verifies Resolve applies local over global over default.
//
// Test_Resolve covers the three independent keys across the three priority levels. For each key,
// local wins when set, then global, then the default. Unset fields (nil) fall through.
//
// @param           t   "test handle used for assertions"
func Test_Resolve(t *testing.T) {
	ten := 10
	twenty := 20
	thirty := 30
	forty := 40
	globalGit := "/usr/bin/git"
	localGit := "/opt/git/bin/git"

	tests := []struct {
		name          string
		global, local *Settings
		wantSync      time.Duration
		wantDebounce  time.Duration
		wantGit       string
	}{
		{"all default", &Settings{}, &Settings{}, DefaultSyncInterval, DefaultDebounce, DefaultGitExec},
		{"global sync", &Settings{SyncInterval: &twenty}, &Settings{}, 20 * time.Minute, DefaultDebounce, DefaultGitExec},
		{"local overrides global sync", &Settings{SyncInterval: &twenty}, &Settings{SyncInterval: &thirty}, 30 * time.Minute, DefaultDebounce, DefaultGitExec},
		{"global debounce", &Settings{Debounce: &ten}, &Settings{}, DefaultSyncInterval, 10 * time.Minute, DefaultGitExec},
		{"local overrides global debounce", &Settings{Debounce: &ten}, &Settings{Debounce: &forty}, DefaultSyncInterval, 40 * time.Minute, DefaultGitExec},
		{"global gitexec", &Settings{GitExec: &globalGit}, &Settings{}, DefaultSyncInterval, DefaultDebounce, globalGit},
		{"local overrides global gitexec", &Settings{GitExec: &globalGit}, &Settings{GitExec: &localGit}, DefaultSyncInterval, DefaultDebounce, localGit},
		{"local only sync", &Settings{}, &Settings{SyncInterval: &thirty}, 30 * time.Minute, DefaultDebounce, DefaultGitExec},
		{"local only gitexec", &Settings{}, &Settings{GitExec: &localGit}, DefaultSyncInterval, DefaultDebounce, localGit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sync, debounce, gitExec := Resolve(tc.global, tc.local)
			if sync != tc.wantSync {
				t.Fatalf("got %v, want %v", sync, tc.wantSync)
			}
			if debounce != tc.wantDebounce {
				t.Fatalf("got %v, want %v", debounce, tc.wantDebounce)
			}
			if gitExec != tc.wantGit {
				t.Fatalf("got %v, want %v", gitExec, tc.wantGit)
			}
		})
	}
}

// @description    Verifies Resolve tolerates nil inputs.
//
// Test_ResolveNils verifies that nil global and/or local produce the defaults.
//
// @param           t   "test handle used for assertions"
func Test_ResolveNils(t *testing.T) {
	sync, debounce, gitExec := Resolve(nil, nil)
	if sync != DefaultSyncInterval {
		t.Fatalf("got %v, want %v", sync, DefaultSyncInterval)
	}
	if debounce != DefaultDebounce {
		t.Fatalf("got %v, want %v", debounce, DefaultDebounce)
	}
	if gitExec != DefaultGitExec {
		t.Fatalf("got %v, want %v", gitExec, DefaultGitExec)
	}

	v := 30
	sync, _, _ = Resolve(nil, &Settings{SyncInterval: &v})
	if sync != 30*time.Minute {
		t.Fatalf("got %v, want %v", sync, 30*time.Minute)
	}
}

// @description    Verifies LocalFingerprint changes only when [auto-sync] keys change.
//
// Test_LocalFingerprint verifies that equal settings produce equal fingerprints, and that changing
// any one of the three keys produces a different fingerprint. A nil settings produces a stable
// empty fingerprint.
//
// @param           t   "test handle used for assertions"
func Test_LocalFingerprint(t *testing.T) {
	if LocalFingerprint(nil) != LocalFingerprint(&Settings{}) {
		t.Fatalf("got %v, want %v", LocalFingerprint(nil), LocalFingerprint(&Settings{}))
	}

	a := 30
	b := 5
	gitA := "/usr/bin/git"
	base := &Settings{SyncInterval: &a, Debounce: &b, GitExec: &gitA}
	fp := LocalFingerprint(base)
	if fp != LocalFingerprint(&Settings{SyncInterval: &a, Debounce: &b, GitExec: &gitA}) {
		t.Fatalf("got %v, want %v", fp, LocalFingerprint(&Settings{SyncInterval: &a, Debounce: &b, GitExec: &gitA}))
	}

	changed := 40
	if LocalFingerprint(&Settings{SyncInterval: &changed, Debounce: &b, GitExec: &gitA}) == fp {
		t.Fatalf("assertion failed: LocalFingerprint(&Settings{SyncInterval: &changed, Debounce: &b, GitExec: &gitA}) != fp")
	}
	if LocalFingerprint(&Settings{SyncInterval: &a, Debounce: &changed, GitExec: &gitA}) == fp {
		t.Fatalf("assertion failed: LocalFingerprint(&Settings{SyncInterval: &a, Debounce: &changed, GitExec: &gitA}) != fp")
	}
	gitB := "/opt/git/bin/git"
	if LocalFingerprint(&Settings{SyncInterval: &a, Debounce: &b, GitExec: &gitB}) == fp {
		t.Fatalf("assertion failed: LocalFingerprint(&Settings{SyncInterval: &a, Debounce: &b, GitExec: &gitB}) != fp")
	}
}
