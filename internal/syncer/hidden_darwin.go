//go:build darwin

package syncer

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// @description    Reports whether a path is hidden by the macOS file flag.
//
// isHiddenByOS reports whether filePath itself or any ancestor directory up to but excluding the
// repository root carries the BSD UF_HIDDEN flag, which macOS Finder and Open dialogs honor. Paths
// at the repository root, outside the repository, or that cannot be queried (deleted, inaccessible)
// are treated as not hidden so the repository root and removal or rename events are never blocked.
//
// @param           repoRoot   "absolute path to the repository root"
//
// @param           filePath   "absolute or repository-relative path within the repository to inspect"
//
// @return          bool       "true when the file or an ancestor directory carries the macOS hidden flag"
func isHiddenByOS(repoRoot string, filePath string) bool {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}

	cur := root
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		cur = filepath.Join(cur, part)
		if hasHiddenFlag(cur) {
			return true
		}
	}
	return false
}

// @description    Reads the macOS file flags of a path and tests the hidden bit.
//
// hasHiddenFlag returns true when the path exists and its file flags include UF_HIDDEN. A path that
// does not exist or cannot be queried reports not hidden so removal and rename events for vanished
// paths are not blocked.
//
// @param           path   "filesystem path to inspect"
//
// @return          bool   "true when the existing path carries the macOS hidden flag"
func hasHiddenFlag(path string) bool {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return false
	}

	return stat.Flags&unix.UF_HIDDEN != 0
}
