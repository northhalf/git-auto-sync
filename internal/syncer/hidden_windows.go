//go:build windows

package syncer

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// @description    Reports whether a path is hidden by the Windows filesystem attribute.
//
// isHiddenByOS reports whether filePath itself or any ancestor directory up to but excluding the
// repository root carries the FILE_ATTRIBUTE_HIDDEN bit. Paths at the repository root, outside the
// repository, or that cannot be queried (deleted, inaccessible) are treated as not hidden so the
// repository root and removal or rename events are never blocked.
//
// @param           repoRoot   "absolute path to the repository root"
//
// @param           filePath   "absolute or repository-relative path within the repository to inspect"
//
// @return          bool       "true when the file or an ancestor directory carries the Windows hidden attribute"
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
		if hasHiddenAttribute(cur) {
			return true
		}
	}
	return false
}

// @description    Reads the Windows file attributes of a path and tests the hidden bit.
//
// hasHiddenAttribute returns true when the path exists and its file attributes include
// FILE_ATTRIBUTE_HIDDEN. A path that does not exist or cannot be queried reports not hidden so
// removal and rename events for vanished paths are not blocked.
//
// @param           path   "filesystem path to inspect"
//
// @return          bool   "true when the existing path carries the Windows hidden attribute"
func hasHiddenAttribute(path string) bool {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}

	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return false
	}

	return attrs&windows.FILE_ATTRIBUTE_HIDDEN != 0
}
