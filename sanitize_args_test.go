package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

// @description    Exercises sanitizeArgs across Termux-duplicate and normal argv shapes.
func TestSanitizeArgs(t *testing.T) {
	tmp := t.TempDir()
	exe := filepath.Join(tmp, "git-auto-sync")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "gas-link")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(tmp, "other")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		argv []string
		exe  string
		want []string
	}{
		{
			name: "termux duplicate with subcommand stripped",
			argv: []string{exe, exe, "sync"},
			exe:  exe,
			want: []string{exe, "sync"},
		},
		{
			name: "termux duplicate without subcommand stripped",
			argv: []string{exe, exe},
			exe:  exe,
			want: []string{exe},
		},
		{
			name: "termux duplicate via symlink stripped",
			argv: []string{exe, link, "sync"},
			exe:  exe,
			want: []string{exe, "sync"},
		},
		{
			name: "normal subcommand unchanged",
			argv: []string{exe, "sync"},
			exe:  exe,
			want: []string{exe, "sync"},
		},
		{
			name: "single argv element unchanged",
			argv: []string{exe},
			exe:  exe,
			want: []string{exe},
		},
		{
			name: "version flag unchanged",
			argv: []string{exe, "--version"},
			exe:  exe,
			want: []string{exe, "--version"},
		},
		{
			name: "unrelated existing file unchanged",
			argv: []string{exe, other, "sync"},
			exe:  exe,
			want: []string{exe, other, "sync"},
		},
		{
			name: "nonexistent command path unchanged",
			argv: []string{exe, "sync", "extra"},
			exe:  exe,
			want: []string{exe, "sync", "extra"},
		},
		{
			name: "empty exe leaves argv unchanged",
			argv: []string{exe, exe, "sync"},
			exe:  "",
			want: []string{exe, exe, "sync"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeArgs(tc.argv, tc.exe)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("sanitizeArgs(%v, %q) = %v, want %v", tc.argv, tc.exe, got, tc.want)
			}
		})
	}
}

// @description    Verifies sanitizeArgs restores urfave/cli command dispatch under Termux's duplicated argv.
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
	argv := sanitizeArgs([]string{exe, exe, "sync"}, exe)
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
