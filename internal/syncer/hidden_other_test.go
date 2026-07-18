//go:build !windows && !darwin

package syncer

import (
	"path/filepath"
	"testing"

	"gotest.tools/v3/assert"
)

// @description    Verifies the no-op hidden-attribute stub is inert.
//
// Test_IsHiddenByOS_StubReturnsFalse verifies that isHiddenByOS reports not hidden for an ordinary
// path on platforms without a filesystem hidden flag, where it is a no-op.
//
// @param           t   "test handle used for path construction and assertion"
func Test_IsHiddenByOS_StubReturnsFalse(t *testing.T) {
	repo := t.TempDir()
	assert.Equal(t, isHiddenByOS(repo, filepath.Join(repo, "src", "file.go")), false)
}
