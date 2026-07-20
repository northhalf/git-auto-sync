package termux

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// @description    Exercises SanitizeArgs across Termux-duplicate and normal argv shapes.
//
// @param           t  "test handle used for table-driven argv assertions"
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
			got := SanitizeArgs(tc.argv)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("SanitizeArgs(%v) = %v, want %v", tc.argv, got, tc.want)
			}
		})
	}

	// Relative argv0 requires cwd to be the binary's directory so "./git-auto-sync"
	// resolves to the same file as the absolute argv1 (matches the reporter's case).
	t.Run("termux relative argv0 absolute argv1 stripped", func(t *testing.T) {
		t.Chdir(tmp)
		argv := []string{"./git-auto-sync", exe, "sync"}
		want := []string{"./git-auto-sync", "sync"}
		got := SanitizeArgs(argv)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("SanitizeArgs(%v) = %v, want %v", argv, got, want)
		}
	})
}
