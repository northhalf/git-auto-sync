//go:build !windows && !darwin

package syncer

// @description    No-op hidden-attribute check for platforms without a filesystem hidden flag.
//
// isHiddenByOS always reports not hidden on platforms that do not carry a Windows hidden attribute
// or macOS hidden file flag, such as Linux. Hidden filtering there relies on the dot-prefix
// convention instead.
//
// @param           repoRoot   "absolute path to the repository root"
//
// @param           filePath   "absolute or repository-relative path within the repository to inspect"
//
// @return          bool       "always false on platforms without a filesystem hidden flag"
func isHiddenByOS(repoRoot string, filePath string) bool {
	return false
}
