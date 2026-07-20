// Package termux centralizes the exec workarounds the Termux (Android) environment requires.
//
// Termux runs every dynamic binary through the Android dynamic linker: termux-exec
// (LD_PRELOAD) rewrites execve(prog, [argv0, args...]) into execve(linker64, [prog, argv0,
// args...]), so /proc/self/exe names linker64 instead of the program, and affected Termux
// versions insert the resolved program path as an extra argv element
// (termux/termux-app#4630). Go binaries built with CGO_ENABLED=0 bypass termux-exec and
// issue raw syscalls, which adds a second problem: Termux ships some binaries (Git, runit's
// sv) as static PIE without a PT_INTERP segment, and the Android kernel refuses a direct
// execve of those with EACCES. Invoking linker64 with the program path as its argument
// mirrors the termux-exec workaround, so Command routes subprocesses through it, ProgramPath
// recovers the real program path, and SanitizeArgs drops the inserted argv duplicate.
package termux

import (
	"os"
	"os/exec"
	"path/filepath"
)

// @description    Builds a command that survives the Termux static-PIE exec restriction.
//
// Command is intended for Android call sites, which gate on the platform themselves (a
// runtime check or the android build tag). It execs the program directly unless the current
// process itself runs under the Android dynamic linker (os.Executable reports linker64), in
// which case it routes the exec through that linker, mirroring the termux-exec workaround
// for Go binaries whose raw syscalls bypass it.
//
// @param           name      "program name or path"
//
// @param           args      "program arguments"
//
// @return          *exec.Cmd "configured command ready to run"
func Command(name string, args ...string) *exec.Cmd {
	self, err := os.Executable()
	if err == nil {
		if cmd, ok := linkerCommand(self, name, args); ok {
			return cmd
		}
	}
	return exec.Command(name, args...)
}

// @description    Wraps a program invocation through the Android dynamic linker.
//
// linkerCommand reports ok == false when selfPath is not the dynamic linker. Otherwise it
// builds a linker invocation with the program as the linker's first argument, resolving a
// non-absolute name through PATH because the linker does not search it. A name that fails
// resolution is passed unresolved, which fails the same way a direct exec would have.
//
// @param           selfPath  "path of the current executable, from os.Executable"
//
// @param           name      "program name or path to execute through the linker"
//
// @param           args      "program arguments"
//
// @return          *exec.Cmd "linker command running the program"
//
// @return          bool      "true when selfPath is the Android dynamic linker"
func linkerCommand(selfPath, name string, args []string) (*exec.Cmd, bool) {
	if filepath.Base(selfPath) != "linker64" {
		return nil, false
	}
	program := name
	if !filepath.IsAbs(program) {
		if resolved, err := exec.LookPath(program); err == nil {
			program = resolved
		}
	}
	return exec.Command(selfPath, append([]string{program}, args...)...), true
}
