package termux

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// @description    Verifies a non-linker self path never produces a linker command.
//
// @param           t  "test handle used for assertions"
func TestLinkerCommandRejectsNonLinkerSelf(t *testing.T) {
	if _, ok := linkerCommand("/usr/bin/git-auto-sync", "git", []string{"status"}); ok {
		t.Fatal("linkerCommand ok = true for a non-linker selfPath, want false")
	}
}

// @description    Verifies an absolute program path wraps through the linker unchanged.
//
// @param           t  "test handle used for assertions"
func TestLinkerCommandWrapsAbsoluteProgram(t *testing.T) {
	linker := filepath.Join(string(filepath.Separator), "apex", "com.android.runtime", "bin", "linker64")
	program := filepath.Join(string(filepath.Separator), "data", "data", "com.termux", "files", "usr", "bin", "git")

	cmd, ok := linkerCommand(linker, program, []string{"status"})
	if !ok {
		t.Fatal("linkerCommand ok = false for a linker64 selfPath, want true")
	}
	want := []string{linker, program, "status"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("linkerCommand args = %v, want %v", cmd.Args, want)
	}
}

// @description    Verifies a bare program name is resolved through PATH before wrapping.
//
// The dynamic linker does not search PATH, so the wrapped invocation must carry the
// resolved absolute path.
//
// @param           t  "test handle used for assertions"
func TestLinkerCommandResolvesBareProgram(t *testing.T) {
	dir := t.TempDir()
	name := "git-auto-sync-test-prog"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	program := filepath.Join(dir, name)
	if err := os.WriteFile(program, []byte("x"), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) returned error %v", program, err)
	}
	t.Setenv("PATH", dir)

	linker := filepath.Join(string(filepath.Separator), "apex", "linker64")
	cmd, ok := linkerCommand(linker, "git-auto-sync-test-prog", []string{"status"})
	if !ok {
		t.Fatal("linkerCommand ok = false for a linker64 selfPath, want true")
	}
	want := []string{linker, program, "status"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("linkerCommand args = %v, want %v", cmd.Args, want)
	}
}

// @description    Verifies an unresolvable program name is passed to the linker unresolved.
//
// @param           t  "test handle used for assertions"
func TestLinkerCommandPassesUnresolvedProgramThrough(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	linker := filepath.Join(string(filepath.Separator), "apex", "linker64")

	cmd, ok := linkerCommand(linker, "git-auto-sync-missing-prog", []string{"status"})
	if !ok {
		t.Fatal("linkerCommand ok = false for a linker64 selfPath, want true")
	}
	want := []string{linker, "git-auto-sync-missing-prog", "status"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("linkerCommand args = %v, want %v", cmd.Args, want)
	}
}

// @description    Verifies Command execs directly on non-Android platforms.
//
// @param           t  "test handle used for assertions"
func TestCommandDirectOffAndroid(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("direct-exec path requires a non-Android platform")
	}
	cmd := Command("git-auto-sync-missing-prog", "a", "b")
	want := []string{"git-auto-sync-missing-prog", "a", "b"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("Command args = %v, want %v", cmd.Args, want)
	}
}
