package daemonservice

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// @description    Creates a file with the given content and mode.
//
// @param           t        "test handle used to create the temporary file"
//
// @param           name     "file name inside the test temporary directory"
//
// @param           content  "file contents"
//
// @param           mode     "permission bits applied to the file"
//
// @return          string   "absolute path of the created file"
func writeExecutableFile(t *testing.T, name, content string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("WriteFile(%q) returned error %v", path, err)
	}
	return path
}

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
	return writeExecutableFile(t, name, "program", mode)
}

// @description    Returns a lookPath stub that fails the test when called.
//
// @param           t  "test handle used to report an unexpected call"
//
// @return          func(string) (string, error)  "lookPath replacement that must not run"
func forbiddenLookPath(t *testing.T) func(string) (string, error) {
	t.Helper()
	return func(name string) (string, error) {
		t.Fatalf("lookPath(%q) must not be called", name)
		return "", nil
	}
}

// @description    Verifies a non-linker executable path is returned unchanged.
//
// @param           t  "test handle used for assertions"
func TestResolveProgramPathReturnsNativeExecutable(t *testing.T) {
	got, err := resolveProgramPath("/usr/local/bin/git-auto-sync", []string{"git-auto-sync", "daemon", "ls"}, forbiddenLookPath(t))
	if err != nil {
		t.Fatalf("resolveProgramPath returned error %v, want nil", err)
	}
	if got != "/usr/local/bin/git-auto-sync" {
		t.Fatalf("resolveProgramPath = %q, want %q", got, "/usr/local/bin/git-auto-sync")
	}
}

// @description    Verifies the Termux-inserted argv[1] duplicate is preferred under linker64.
//
// @param           t  "test handle used for assertions"
func TestResolveProgramPathPrefersTermuxArgvDuplicate(t *testing.T) {
	program := writeProgramFile(t, "git-auto-sync", 0o755)
	lookPath := func(string) (string, error) {
		return "/should/not/be/used/git-auto-sync", nil
	}
	got, err := resolveProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", program, "daemon", "ls"}, lookPath)
	if err != nil {
		t.Fatalf("resolveProgramPath returned error %v, want nil", err)
	}
	if got != program {
		t.Fatalf("resolveProgramPath = %q, want %q", got, program)
	}
}

// @description    Verifies PATH lookup resolves argv[0] when argv[1] is a real argument.
//
// @param           t  "test handle used for assertions"
func TestResolveProgramPathFallsBackToPathLookup(t *testing.T) {
	program := writeProgramFile(t, "git-auto-sync", 0o755)
	lookPath := func(name string) (string, error) {
		if name != "git-auto-sync" {
			t.Fatalf("lookPath name = %q, want %q", name, "git-auto-sync")
		}
		return program, nil
	}
	got, err := resolveProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", "daemon", "ls"}, lookPath)
	if err != nil {
		t.Fatalf("resolveProgramPath returned error %v, want nil", err)
	}
	if got != program {
		t.Fatalf("resolveProgramPath = %q, want %q", got, program)
	}
}

// @description    Verifies a non-executable argv[1] duplicate candidate is skipped.
//
// @param           t  "test handle used for assertions"
func TestResolveProgramPathSkipsNonExecutableDuplicate(t *testing.T) {
	program := writeProgramFile(t, "git-auto-sync", 0o644)
	resolved := writeProgramFile(t, "git-auto-sync", 0o755)
	lookPath := func(string) (string, error) {
		return resolved, nil
	}
	got, err := resolveProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", program, "daemon"}, lookPath)
	if err != nil {
		t.Fatalf("resolveProgramPath returned error %v, want nil", err)
	}
	if got != resolved {
		t.Fatalf("resolveProgramPath = %q, want %q", got, resolved)
	}
}

// @description    Verifies an argv[1] whose basename differs from argv[0] is skipped.
//
// @param           t  "test handle used for assertions"
func TestResolveProgramPathSkipsMismatchedDuplicate(t *testing.T) {
	other := writeProgramFile(t, "other-tool", 0o755)
	resolved := writeProgramFile(t, "git-auto-sync", 0o755)
	lookPath := func(string) (string, error) {
		return resolved, nil
	}
	got, err := resolveProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", other}, lookPath)
	if err != nil {
		t.Fatalf("resolveProgramPath returned error %v, want nil", err)
	}
	if got != resolved {
		t.Fatalf("resolveProgramPath = %q, want %q", got, resolved)
	}
}

// @description    Verifies an empty argv under linker64 reports a resolution error.
//
// @param           t  "test handle used for assertions"
func TestResolveProgramPathEmptyArgv(t *testing.T) {
	_, err := resolveProgramPath("/apex/com.android.runtime/bin/linker64", nil, forbiddenLookPath(t))
	if err == nil {
		t.Fatal("resolveProgramPath returned nil error, want a resolution error")
	}
}

// @description    Verifies a PATH lookup failure under linker64 is wrapped and returned.
//
// @param           t  "test handle used for assertions"
func TestResolveProgramPathLookupFailure(t *testing.T) {
	lookErr := errors.New("no such executable")
	lookPath := func(string) (string, error) {
		return "", lookErr
	}
	_, err := resolveProgramPath("/apex/com.android.runtime/bin/linker64", []string{"git-auto-sync", "daemon"}, lookPath)
	if !errors.Is(err, lookErr) {
		t.Fatalf("resolveProgramPath error = %v, want it to wrap %v", err, lookErr)
	}
}

// @description    Verifies a directly executable program runs without invoking the linker.
//
// @param           t  "test handle used for assertions"
func TestRunWithLinkerRetryDirectSuccess(t *testing.T) {
	program := writeExecutableFile(t, "prog", "#!/bin/sh\necho direct-ok\n", 0o755)
	output, err := runWithLinkerRetry("/usr/bin/git-auto-sync", program)
	if err != nil {
		t.Fatalf("runWithLinkerRetry returned error %v, want nil", err)
	}
	if strings.TrimSpace(string(output)) != "direct-ok" {
		t.Fatalf("runWithLinkerRetry output = %q, want %q", output, "direct-ok")
	}
}

// @description    Verifies an EACCES failure is returned unchanged without a linker self path.
//
// A file without any execute bit fails execve with EACCES even for the root user.
//
// @param           t  "test handle used for assertions"
func TestRunWithLinkerRetryWithoutLinkerReturnsDirectError(t *testing.T) {
	program := writeExecutableFile(t, "prog", "#!/bin/sh\necho unreachable\n", 0o644)
	_, err := runWithLinkerRetry("/usr/bin/git-auto-sync", program)
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("runWithLinkerRetry error = %v, want fs.ErrPermission", err)
	}
}

// @description    Verifies an EACCES failure is retried through the linker when self is linker64.
//
// The fake linker stands in for the Android linker64 behavior of loading a program the kernel
// refused to execve directly: it interprets the target with sh instead.
//
// @param           t  "test handle used for assertions"
func TestRunWithLinkerRetryRetriesThroughLinker(t *testing.T) {
	linker := writeExecutableFile(t, "linker64", "#!/bin/sh\nexec /bin/sh \"$@\"\n", 0o755)
	program := writeExecutableFile(t, "prog", "echo linker-ok\n", 0o644)
	output, err := runWithLinkerRetry(linker, program)
	if err != nil {
		t.Fatalf("runWithLinkerRetry returned error %v, want nil", err)
	}
	if strings.TrimSpace(string(output)) != "linker-ok" {
		t.Fatalf("runWithLinkerRetry output = %q, want %q", output, "linker-ok")
	}
}

// @description    Verifies a linker retry failure reports the direct and linker errors.
//
// @param           t  "test handle used for assertions"
func TestRunWithLinkerRetryLinkerFailure(t *testing.T) {
	linker := writeExecutableFile(t, "linker64", "#!/bin/sh\nexit 1\n", 0o755)
	program := writeExecutableFile(t, "prog", "echo unreachable\n", 0o644)
	_, err := runWithLinkerRetry(linker, program)
	if err == nil {
		t.Fatal("runWithLinkerRetry returned nil error, want a retry failure")
	}
	if !strings.Contains(err.Error(), "linker retry failed") {
		t.Fatalf("runWithLinkerRetry error = %v, want it to mention the linker retry", err)
	}
}
