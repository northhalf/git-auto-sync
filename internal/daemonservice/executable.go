package daemonservice

import (
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
)

// @description    Resolves the running program path when the OS reports a wrapper.
//
// On Termux, os.Executable returns the Android dynamic linker (linker64): termux-exec
// re-execs every dynamic binary through the linker, so /proc/self/exe names the linker
// instead of the program. resolveProgramPath detects that case and recovers the real
// program path from argv: affected Termux versions insert the resolved program path as
// argv[1] (termux/termux-app#4630), and argv[0] otherwise re-resolves through PATH the
// same way termux-exec resolved it. Any other rawExecutable is returned unchanged.
//
// @param           rawExecutable  "path reported by os.Executable"
//
// @param           args           "process arguments, typically os.Args"
//
// @param           lookPath       "PATH resolution function, typically exec.LookPath"
//
// @return          string         "path of the running program"
//
// @return          error          "resolution failure when the linker wrapper hides the program path"
func resolveProgramPath(rawExecutable string, args []string, lookPath func(string) (string, error)) (string, error) {
	if filepath.Base(rawExecutable) != "linker64" {
		return rawExecutable, nil
	}
	if len(args) > 1 && filepath.IsAbs(args[1]) && filepath.Base(args[0]) == filepath.Base(args[1]) {
		if err := requireExecutable(args[1]); err == nil {
			return args[1], nil
		}
	}
	if len(args) == 0 {
		return "", fmt.Errorf("os.Executable returned the Android dynamic linker %q and argv is empty", rawExecutable)
	}
	resolved, err := lookPath(args[0])
	if err != nil {
		return "", fmt.Errorf("os.Executable returned the Android dynamic linker and %q could not be resolved through PATH: %w", args[0], err)
	}
	return resolved, nil
}

// @description    Executes a program, retrying through the Android dynamic linker.
//
// Termux ships some binaries (runit's sv, Git) as static PIE without a PT_INTERP segment,
// so the Android kernel refuses a direct execve with EACCES. Shell-launched processes
// succeed because termux-exec (LD_PRELOAD) rewrites the exec through linker64, but Go
// binaries built with CGO_ENABLED=0 issue raw syscalls and bypass that interception.
// runWithLinkerRetry therefore retries an EACCES failure through the linker when the
// current process itself runs under it, mirroring the termux-exec workaround.
//
// @param           selfPath  "path of the current executable, from os.Executable"
//
// @param           path      "program path to execute"
//
// @param           args      "program arguments"
//
// @return          []byte    "combined standard output and standard error"
//
// @return          error     "nil on success, or the direct or linker execution error"
func runWithLinkerRetry(selfPath, path string, args ...string) ([]byte, error) {
	output, err := exec.Command(path, args...).CombinedOutput()
	if !errors.Is(err, fs.ErrPermission) {
		return output, err
	}
	if filepath.Base(selfPath) != "linker64" {
		return output, err
	}
	linkerArgs := append([]string{path}, args...)
	linkerOutput, linkerErr := exec.Command(selfPath, linkerArgs...).CombinedOutput()
	if linkerErr != nil {
		return linkerOutput, fmt.Errorf("direct exec failed with %v, linker retry failed: %w", err, linkerErr)
	}
	return linkerOutput, nil
}
