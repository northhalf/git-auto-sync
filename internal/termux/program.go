package termux

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// @description    Resolves the running program path when the OS reports a wrapper.
//
// On Termux, os.Executable returns the Android dynamic linker (linker64): termux-exec
// re-execs every dynamic binary through the linker, so /proc/self/exe names the linker
// instead of the program. ProgramPath detects that case and recovers the real program path
// from argv: affected Termux versions insert the resolved program path as argv[1]
// (termux/termux-app#4630), and argv[0] otherwise re-resolves through PATH the same way
// termux-exec resolved it. Any other rawExecutable is returned unchanged.
//
// @param           rawExecutable  "path reported by os.Executable"
//
// @param           args           "process arguments, typically os.Args"
//
// @return          string         "path of the running program"
//
// @return          error          "resolution failure when the linker wrapper hides the program path"
func ProgramPath(rawExecutable string, args []string) (string, error) {
	if filepath.Base(rawExecutable) != "linker64" {
		return rawExecutable, nil
	}
	if len(args) > 1 && filepath.IsAbs(args[1]) && filepath.Base(args[0]) == filepath.Base(args[1]) {
		if err := RequireExecutable(args[1]); err == nil {
			return args[1], nil
		}
	}
	if len(args) == 0 {
		return "", fmt.Errorf("os.Executable returned the Android dynamic linker %q and argv is empty", rawExecutable)
	}
	resolved, err := exec.LookPath(args[0])
	if err != nil {
		return "", fmt.Errorf("os.Executable returned the Android dynamic linker and %q could not be resolved through PATH: %w", args[0], err)
	}
	return resolved, nil
}

// @description    Checks that a path names an executable regular file.
//
// @param           path  "filesystem path to validate"
//
// @return          error  "nil for an executable file, or a validation error"
func RequireExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", path)
	}
	return nil
}
