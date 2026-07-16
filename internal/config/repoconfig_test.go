package config

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"gotest.tools/v3/assert"
)

// @description    Verifies the default filesystem debounce duration.
//
// Test_NewRepoConfigDefaultFSLag verifies that repositories without an explicit auto-sync timing
// configuration wait ten minutes after the latest file event before requesting a synchronization.
//
// @param           t   "test handle used to create the repository and assert its configuration"
func Test_NewRepoConfigDefaultFSLag(t *testing.T) {
	repoPath := t.TempDir()
	_, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	cfg, err := NewRepoConfig(repoPath)
	assert.NilError(t, err)
	assert.Equal(t, cfg.FSLag, 10*time.Minute)
}
