package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	cfg "github.com/northhalf/git-auto-sync/internal/config"
	"github.com/urfave/cli/v2"
)

// @description    Isolates the global config directory for the test.
//
// configEnv points XDG_CONFIG_HOME and HOME at a fresh temp dir. Call once per test; runConfigIn
// then shares that directory across multiple command invocations so set-then-read sequences see
// earlier writes.
//
// @param           t   "test handle used to isolate the configuration directory"
func configEnv(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

// @description    Invokes the config command with args against the current environment.
//
// runConfigIn runs the config command in an in-memory cli.App and captures combined output. It does
// not change the working directory; tests that need a repository chdir first (see initRepo).
//
// @param           t        "test handle used for the in-memory app"
//
// @param           args     "arguments forwarded to the config command"
//
// @return          string   "captured combined output"
//
// @return          error    "error returned by the config command"
func runConfigIn(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	app := &cli.App{
		Writer:    &out,
		ErrWriter: &out,
		Commands: []*cli.Command{
			{
				Name:  "config",
				Usage: "Get, set, or unset git-auto-sync settings",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "global", Usage: "Operate on the global config.json"},
					&cli.BoolFlag{Name: "local", Usage: "Operate on the repository's .git/config"},
					&cli.BoolFlag{Name: "get", Usage: "Print the effective value of a key"},
					&cli.BoolFlag{Name: "list", Aliases: []string{"l"}, Usage: "List all settings"},
					&cli.BoolFlag{Name: "unset", Aliases: []string{"u"}, Usage: "Remove a key"},
				},
				Action: configCmd,
			},
		},
	}
	err := app.Run(append([]string{"git-auto-sync", "config"}, args...))
	return out.String(), err
}

// @description    Creates a temp git repo, chdirs into it, and returns its path.
//
// initRepo changes the working directory for the test; t.Chdir restores the original directory on
// cleanup.
//
// @param           t        "test handle used for the temp directory and chdir"
//
// @return          string   "absolute path to the created repository"
func initRepo(t *testing.T) string {
	t.Helper()
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Chdir(repoPath)
	return repoPath
}

// @description    Verifies a default-scope write goes to global config.
//
// Test_ConfigWriteDefaultGlobal sets syncInterval with no scope flag and reads it back from global.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigWriteDefaultGlobal(t *testing.T) {
	configEnv(t)
	_, err := runConfigIn(t, "syncInterval", "90")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s, err := cfg.ReadGlobalSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *s.SyncInterval != 90 {
		t.Fatalf("got %v, want %v", *s.SyncInterval, 90)
	}
}

// @description    Verifies --global write.
//
// Test_ConfigWriteGlobal sets debounce with --global and reads it back.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigWriteGlobal(t *testing.T) {
	configEnv(t)
	_, err := runConfigIn(t, "--global", "debounce", "7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s, err := cfg.ReadGlobalSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *s.Debounce != 7 {
		t.Fatalf("got %v, want %v", *s.Debounce, 7)
	}
}

// @description    Verifies --local write.
//
// Test_ConfigWriteLocal sets gitexec with --local in a repo and reads it back.
//
// @param           t   "test handle used to create the repo and assert"
func Test_ConfigWriteLocal(t *testing.T) {
	configEnv(t)
	repoPath := initRepo(t)
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = runConfigIn(t, "--local", "gitexec", gitPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	local, err := cfg.ReadLocalSettings(repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *local.GitExec != gitPath {
		t.Fatalf("got %v, want %v", *local.GitExec, gitPath)
	}
}

// @description    Verifies effective read with no scope.
//
// Test_ConfigReadEffective reads syncInterval and expects the default when unset, then sets a
// global value and reads it back in a second invocation sharing the same config directory.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigReadEffective(t *testing.T) {
	configEnv(t)

	out, err := runConfigIn(t, "syncInterval")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "60") {
		t.Fatalf("assertion failed: strings.Contains(out, \"60\")")
	}

	_, err = runConfigIn(t, "--global", "syncInterval", "45")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err = runConfigIn(t, "syncInterval")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "45") {
		t.Fatalf("assertion failed: strings.Contains(out, \"45\")")
	}
}

// @description    Verifies --local read shows only the local value.
//
// Test_ConfigReadLocal reads a local syncInterval and expects only that value, with no output when
// unset.
//
// @param           t   "test handle used to create the repo and assert"
func Test_ConfigReadLocal(t *testing.T) {
	configEnv(t)
	repoPath := initRepo(t)

	out, err := runConfigIn(t, "--local", "syncInterval")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("got %v, want %v", strings.TrimSpace(out), "")
	}

	if err := cfg.SetLocalSetting(repoPath, "syncInterval", "33"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err = runConfigIn(t, "--local", "syncInterval")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "33") {
		t.Fatalf("assertion failed: strings.Contains(out, \"33\")")
	}
}

// @description    Verifies --list prints all three effective keys.
//
// Test_ConfigList runs config --list and expects all three key names in the output.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigList(t *testing.T) {
	configEnv(t)
	out, err := runConfigIn(t, "--list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "syncInterval") {
		t.Fatalf("assertion failed: strings.Contains(out, \"syncInterval\")")
	}
	if !strings.Contains(out, "debounce") {
		t.Fatalf("assertion failed: strings.Contains(out, \"debounce\")")
	}
	if !strings.Contains(out, "gitexec") {
		t.Fatalf("assertion failed: strings.Contains(out, \"gitexec\")")
	}
}

// @description    Verifies --unset removes a global key.
//
// Test_ConfigUnsetGlobal sets then unsets a global syncInterval and confirms it is gone.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigUnsetGlobal(t *testing.T) {
	configEnv(t)
	_, err := runConfigIn(t, "syncInterval", "90")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = runConfigIn(t, "--unset", "syncInterval")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s, err := cfg.ReadGlobalSettings()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SyncInterval != nil {
		t.Fatalf("assertion failed: s.SyncInterval == nil")
	}
}

// @description    Verifies an unknown key errors.
//
// Test_ConfigUnknownKey verifies that setting an unknown key returns an error.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigUnknownKey(t *testing.T) {
	configEnv(t)
	_, err := runConfigIn(t, "bogus", "1")
	if err == nil {
		t.Fatalf("assertion failed: err != nil")
	}
}

// @description    Verifies a non-positive interval errors.
//
// Test_ConfigInvalidInterval verifies that zero or negative intervals error.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigInvalidInterval(t *testing.T) {
	configEnv(t)
	_, err := runConfigIn(t, "syncInterval", "0")
	if err == nil {
		t.Fatalf("assertion failed: err != nil")
	}
	_, err = runConfigIn(t, "syncInterval", "-5")
	if err == nil {
		t.Fatalf("assertion failed: err != nil")
	}
}

// @description    Verifies a nonexistent gitexec errors on write.
//
// Test_ConfigGitExecMissing verifies that setting gitexec to a nonexistent path errors.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigGitExecMissing(t *testing.T) {
	configEnv(t)
	_, err := runConfigIn(t, "gitexec", "/does/not/exist/git")
	if err == nil {
		t.Fatalf("assertion failed: err != nil")
	}
}

// @description    Verifies --global and --local together error.
//
// Test_ConfigBothScopes verifies that passing both --global and --local errors.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigBothScopes(t *testing.T) {
	configEnv(t)
	_, err := runConfigIn(t, "--global", "--local", "syncInterval", "10")
	if err == nil {
		t.Fatalf("assertion failed: err != nil")
	}
}

// @description    Verifies --local outside a repo errors.
//
// Test_ConfigLocalOutsideRepo verifies that --local with no repository returns an error.
//
// @param           t   "test handle used to isolate config and assert"
func Test_ConfigLocalOutsideRepo(t *testing.T) {
	configEnv(t)
	t.Chdir(t.TempDir())
	_, err := runConfigIn(t, "--local", "syncInterval", "10")
	if err == nil {
		t.Fatalf("assertion failed: err != nil")
	}
}
