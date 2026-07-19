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
	other := filepath.Join(tmp, "other-exe")
	if err := os.WriteFile(other, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "termux absolute argv0 and argv1 same file stripped",
			argv: []string{exe, exe, "sync"},
			want: []string{exe, "sync"},
		},
		{
			name: "termux symlinked argv1 stripped via SameFile",
			argv: []string{exe, link, "sync"},
			want: []string{exe, "sync"},
		},
		{
			name: "termux PATH bare argv0 with absolute argv1 stripped by basename",
			argv: []string{"git-auto-sync", exe, "sync"},
			want: []string{"git-auto-sync", "sync"},
		},
		{
			name: "normal subcommand unchanged",
			argv: []string{exe, "sync"},
			want: []string{exe, "sync"},
		},
		{
			name: "single argv element unchanged",
			argv: []string{exe},
			want: []string{exe},
		},
		{
			name: "version flag unchanged",
			argv: []string{exe, "--version"},
			want: []string{exe, "--version"},
		},
		{
			name: "unrelated executable unchanged",
			argv: []string{exe, other, "sync"},
			want: []string{exe, other, "sync"},
		},
		{
			name: "nonexistent command path unchanged",
			argv: []string{exe, "sync", "extra"},
			want: []string{exe, "sync", "extra"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeArgs(tc.argv)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("sanitizeArgs(%v) = %v, want %v", tc.argv, got, tc.want)
			}
		})
	}

	// Relative argv0 requires cwd to be the binary's directory so "./git-auto-sync"
	// resolves to the same file as the absolute argv1 (matches the reporter's case).
	t.Run("termux relative argv0 absolute argv1 stripped", func(t *testing.T) {
		t.Chdir(tmp)
		argv := []string{"./git-auto-sync", exe, "sync"}
		want := []string{"./git-auto-sync", "sync"}
		got := sanitizeArgs(argv)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("sanitizeArgs(%v) = %v, want %v", argv, got, want)
		}
	})
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
	argv := sanitizeArgs([]string{exe, exe, "sync"})
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
