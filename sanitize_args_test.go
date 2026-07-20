package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"

	"github.com/northhalf/git-auto-sync/internal/termux"
)

// @description    Verifies SanitizeArgs restores urfave/cli command dispatch under Termux's duplicated argv.
//
// @param           t  "test handle used for dispatch and output assertions"
func TestSanitizeArgsRestoresCommandDispatch(t *testing.T) {
	tmp := t.TempDir()
	exe := filepath.Join(tmp, "git-auto-sync")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	origExiter := cli.OsExiter
	cli.OsExiter = func(int) {}
	defer func() { cli.OsExiter = origExiter }()

	ran := false
	app := &cli.App{
		Name: "git-auto-sync",
		Commands: []*cli.Command{
			{Name: "sync", Action: func(*cli.Context) error { ran = true; return nil }},
		},
		Writer:    &buf,
		ErrWriter: &buf,
	}

	// Simulate Termux's duplicated argv (termux/termux-app#4630): [exe, exe, "sync"].
	argv := termux.SanitizeArgs([]string{exe, exe, "sync"})
	if err := app.Run(argv); err != nil {
		t.Fatalf("app.Run failed: %v (output: %s)", err, buf.String())
	}
	if !ran {
		t.Errorf("sync action did not run; output: %s", buf.String())
	}
	if strings.Contains(buf.String(), "No help topic") {
		t.Errorf("unexpected No help topic output: %s", buf.String())
	}
}
