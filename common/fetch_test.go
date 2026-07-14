package common

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	cp "github.com/otiai10/copy"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gotest.tools/v3/assert"
)

// @description    Prepares related repository fixtures.
//
// PrepareMultiFixtures creates a temporary testdata directory, copies each dependency fixture into
// it, renames each .gitted directory to .git, prepares the named repository fixture, rewrites its
// remote paths to the copied dependencies, and returns its configuration.
//
// @param           t      "test handle used for temporary directories and fixture assertions"
//
// @param           name   "name of the repository fixture to prepare"
//
// @param           deps   "names of dependency fixtures to copy into temporary testdata"
//
// @return          RepoConfig   "configuration for the prepared repository fixture"
func PrepareMultiFixtures(t *testing.T, name string, deps []string) RepoConfig {
	newTestDataPath := t.TempDir()

	for _, name := range deps {
		fixturePath := filepath.Join("testdata", name)
		newPath := filepath.Join(newTestDataPath, name)
		err := cp.Copy(fixturePath, newPath)
		assert.NilError(t, err)

		err = os.Rename(filepath.Join(newPath, ".gitted"), filepath.Join(newPath, ".git"))
		assert.NilError(t, err)
	}

	newRepoConfig := PrepareFixture(t, name)
	FixFixtureGitConfig(t, newRepoConfig.RepoPath, newTestDataPath)

	return newRepoConfig
}

// @description    Rewrites fixture remote paths.
//
// FixFixtureGitConfig rewrites every $TESTDATA$ placeholder in a fixture repository's Git config
// to the temporary testdata path so its remotes use the copied fixtures.
//
// @param           t              "test handle used for file-operation assertions"
//
// @param           newRepoPath    "path to the prepared repository fixture"
//
// @param           testDataPath   "path to the temporary directory containing dependency fixtures"
func FixFixtureGitConfig(t *testing.T, newRepoPath string, testDataPath string) {
	dotGitPath := filepath.Join(newRepoPath, ".git")
	gitConfigFilePath := filepath.Join(dotGitPath, "config")
	input, err := os.ReadFile(gitConfigFilePath)
	assert.NilError(t, err)

	output := bytes.ReplaceAll(input, []byte("$TESTDATA$"), []byte(testDataPath))

	err = os.WriteFile(gitConfigFilePath, output, 0666)
	assert.NilError(t, err)
}

// @description    Verifies remote-tracking updates.
//
// Test_SimpleFetch verifies that fetching leaves the local HEAD unchanged while updating the
// origin1/master remote-tracking reference to the dependency fixture's commit.
//
// @param           t   "test handle used for fixture setup and Git assertions"
func Test_SimpleFetch(t *testing.T) {
	repoConfig := PrepareMultiFixtures(t, "simple_fetch", []string{"multiple_file_change"})

	err := fetch(repoConfig)
	assert.NilError(t, err)

	r, err := git.PlainOpen(repoConfig.RepoPath)
	assert.NilError(t, err)

	head, err := r.Head()
	assert.NilError(t, err)

	assert.Equal(t, head.Hash(), plumbing.NewHash("28cc969d97ddb7640f5e1428bbc8f2947d1ffd57"))

	ref, err := r.Reference(plumbing.NewRemoteReferenceName("origin1", "master"), true)
	assert.NilError(t, err)

	assert.Equal(t, ref.Hash(), plumbing.NewHash("7058b6b292ee3d1382670334b5f29570a1117ef1"))
}
