package syncer

import ()

// @description    Determines whether a file should be ignored.
//
// ShouldIgnoreFile reports whether a path should be excluded from watching and commits. It is a
// single-shot convenience wrapper around NewIgnoreChecker and IgnoreChecker.ShouldIgnore for
// callers that check one path per repository, such as the check CLI command. Callers checking many
// paths in one round should construct an IgnoreChecker once and reuse it.
//
// @param           repoPath  "path to the repository root"
//
// @param           filePath  "absolute or repository-relative path within repoPath to inspect"
//
// @return          bool      "true when the file should be excluded from watching and commits"
//
// @return          error     "nil on success, or an error while inspecting the file or Git rules"
func ShouldIgnoreFile(repoPath string, filePath string) (bool, error) {
	checker, err := NewIgnoreChecker(repoPath)
	if err != nil {
		return false, err
	}
	return checker.ShouldIgnore(filePath)
}
