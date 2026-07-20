//go:build windows || darwin

package syncer

import (
	"path/filepath"
	"strings"
)

// @description    Reports whether a path or an ancestor directory is hidden by the OS.
//
// hasHiddenAncestor walks filePath and each ancestor directory up to but excluding the
// repository root, querying each with the platform's isHidden probe. Paths at the repository
// root, outside the repository, or that fail resolution are treated as not hidden so the
// repository root and removal or rename events are never blocked.
//
// @param           repoRoot   "absolute path to the repository root"
//
// @param           filePath   "absolute or repository-relative path within the repository to inspect"
//
// @param           isHidden   "platform probe reporting whether one path is hidden"
//
// @return          bool       "true when the file or an ancestor directory is hidden"
func hasHiddenAncestor(repoRoot string, filePath string, isHidden func(string) bool) bool {
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
		if isHidden(cur) {
			return true
		}
	}
	return false
}
