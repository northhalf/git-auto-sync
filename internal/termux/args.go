package termux

import (
	"os"
	"path/filepath"
)

// @description    Drops a Termux-inserted self-path duplicate from argv.
//
// On affected Termux versions (termux/termux-app#4630), the exec wrapper rewrites
// execve(prog, [argv0, args...]) to execve(linker64, [prog, argv0, args...]), so
// Go sees os.Args = [argv0, progAbsolutePath, args...]. urfave/cli then treats
// the inserted progAbsolutePath as an unknown command and reports "No help topic
// for '<path>'". os.Executable() returns linker64 (not prog), so SanitizeArgs
// detects the duplicate by comparing argv[1] against argv[0]: when they name the
// same file, argv[1] is the inserted duplicate and is removed.
//
// @param           argv       "raw command-line arguments, typically os.Args"
//
// @return          []string   "argv with a leading self-path duplicate removed, or argv unchanged"
func SanitizeArgs(argv []string) []string {
	if len(argv) < 2 {
		return argv
	}
	if isSelfDuplicate(argv[0], argv[1]) {
		return append([]string{argv[0]}, argv[2:]...)
	}
	return argv
}

// @description    Reports whether argv[1] is the Termux-inserted executable duplicate.
//
// isSelfDuplicate returns true when argv[1] names the same file as argv[0]
// (absolute or relative path invocation), or for PATH invocation where argv[0]
// is a bare name, when argv[1] is an executable whose basename equals argv[0].
//
// @param           argv0  "user-typed program name, os.Args[0]"
//
// @param           argv1  "suspect duplicate, os.Args[1]"
//
// @return          bool   "true when argv[1] is the inserted executable path"
func isSelfDuplicate(argv0, argv1 string) bool {
	if sameExecutablePath(argv0, argv1) {
		return true
	}
	return filepath.Base(argv1) == argv0 && RequireExecutable(argv1) == nil
}

// @description    Reports whether two paths name the same file.
//
// sameExecutablePath compares the paths directly and, when both stat, by inode
// via os.SameFile so that a relative, symlinked, or Android multi-user aliased
// path still matches. A path that does not exist returns false.
//
// @param           a      "first path"
//
// @param           b      "second path"
//
// @return          bool   "true when both paths resolve to the same file"
func sameExecutablePath(a, b string) bool {
	if a == b {
		return true
	}
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(ia, ib)
}
