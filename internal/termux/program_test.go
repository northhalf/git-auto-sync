package termux

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// @description    Creates a file with the given mode for path-resolution tests.
//
// @param           t     "test handle used to create the temporary file"
//
// @param           name  "file name inside the test temporary directory"
//
// @param           mode  "permission bits applied to the file"
//
// @return          string  "absolute path of the created file"
func writeProgramFile(t *testing.T, name string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("program"), mode); err != nil {
		t.Fatalf("WriteFile(%q) returned error %v", path, err)
	}
	return path
}

// @description    Verifies a non-linker executable path is returned unchanged.
//
// @param           t  "test handle used for assertions"
func TestProgramPathReturnsNativeExecutable(t *testing.T) {
	got, err := ProgramPath("/usr/local/bin/git-auto-sync", []string{"git-auto-sync", "daemon", "ls"})
	if err != nil {
		t.Fatalf("ProgramPath returned error %v, want nil", err)
	}
	if got != "/usr/local/bin/git-auto-sync" {
		t.Fatalf("ProgramPath = %q, want %q", got, "/usr/local/bin/git-auto-sync")
	}
}

// @description    Verifies the Termux-inserted argv[1] duplicate is preferred under linker64.
//
// TestProgramPathPrefersTermuxArgvDuplicate leaves PATH empty so a fallthrough to the PATH
// lookup would fail the test with a resolution error instead of silently passing.
//
// @param           t  "test handle used for assertions"
func TestProgramPathPrefersTermuxArgvDuplicate(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	program := writeProgramFile(t, "git-auto-sync", 0o755)
	got, err := ProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", program, "daemon", "ls"})
	if err != nil {
		t.Fatalf("ProgramPath returned error %v, want nil", err)
	}
	if got != program {
		t.Fatalf("ProgramPath = %q, want %q", got, program)
	}
}

// @description    Verifies PATH lookup resolves argv[0] when argv[1] is a real argument.
//
// @param           t  "test handle used for assertions"
func TestProgramPathFallsBackToPathLookup(t *testing.T) {
	program := writeProgramFile(t, "git-auto-sync", 0o755)
	t.Setenv("PATH", filepath.Dir(program))
	got, err := ProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", "daemon", "ls"})
	if err != nil {
		t.Fatalf("ProgramPath returned error %v, want nil", err)
	}
	if got != program {
		t.Fatalf("ProgramPath = %q, want %q", got, program)
	}
}

// @description    Verifies a non-executable argv[1] duplicate candidate is skipped.
//
// @param           t  "test handle used for assertions"
func TestProgramPathSkipsNonExecutableDuplicate(t *testing.T) {
	program := writeProgramFile(t, "git-auto-sync", 0o644)
	resolved := writeProgramFile(t, "git-auto-sync", 0o755)
	t.Setenv("PATH", filepath.Dir(resolved))
	got, err := ProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", program, "daemon"})
	if err != nil {
		t.Fatalf("ProgramPath returned error %v, want nil", err)
	}
	if got != resolved {
		t.Fatalf("ProgramPath = %q, want %q", got, resolved)
	}
}

// @description    Verifies an argv[1] whose basename differs from argv[0] is skipped.
//
// @param           t  "test handle used for assertions"
func TestProgramPathSkipsMismatchedDuplicate(t *testing.T) {
	other := writeProgramFile(t, "other-tool", 0o755)
	resolved := writeProgramFile(t, "git-auto-sync", 0o755)
	t.Setenv("PATH", filepath.Dir(resolved))
	got, err := ProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", other})
	if err != nil {
		t.Fatalf("ProgramPath returned error %v, want nil", err)
	}
	if got != resolved {
		t.Fatalf("ProgramPath = %q, want %q", got, resolved)
	}
}

// @description    Verifies an empty argv under linker64 reports a resolution error.
//
// @param           t  "test handle used for assertions"
func TestProgramPathEmptyArgv(t *testing.T) {
	_, err := ProgramPath("/apex/com.android.runtime/bin/linker64", nil)
	if err == nil {
		t.Fatal("ProgramPath returned nil error, want a resolution error")
	}
}

// @description    Verifies a PATH lookup failure under linker64 is wrapped and returned.
//
// @param           t  "test handle used for assertions"
func TestProgramPathLookupFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := ProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", "daemon"})
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("ProgramPath error = %v, want it to wrap %v", err, exec.ErrNotFound)
	}
}
