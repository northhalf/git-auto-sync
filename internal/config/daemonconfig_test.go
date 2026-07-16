package config

import (
	"testing"

	"gotest.tools/v3/assert"
)

// @description    Prepares an isolated configuration directory.
//
// setup creates a temporary configuration directory, points XDG_CONFIG_HOME and HOME at it for the
// test, and refreshes configdir's cached paths.
//
// @param           t      "test handle used for the temporary directory and environment changes"
//
// @param           name   "descriptive setup name retained for test-call compatibility"
func setup(t *testing.T, _ string) {
	newConfigDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", newConfigDir)
	t.Setenv("HOME", newConfigDir)
}

// @description    Verifies version-one configuration round trips.
//
// Test_SimpleWriteReadV1 verifies that writing a V1 configuration with repository and environment
// entries to an isolated config directory reads back an equal value.
//
// @param           t   "test handle used for isolated configuration setup and assertions"
func Test_SimpleWriteReadV1(t *testing.T) {
	setup(t, "SimpleWriteRead")

	c := &DaemonConfig{
		Repos: []string{"/home/xyz/hello"},
		Envs:  []string{"SSH_AUTH_SOCK=/private/tmp/com.apple.launchd.74ZznY1v1F/Listeners"},
	}
	err := WriteDaemonConfig(c)
	assert.NilError(t, err)

	c2, err := ReadDaemonConfig()
	assert.NilError(t, err)

	assert.DeepEqual(t, c, c2)
}

// @description    Verifies reads without a configuration file.
//
// Test_ReadEmptyV1 verifies that reading from an isolated configuration directory with no V1 file
// succeeds and returns a configuration with no repositories.
//
// @param           t   "test handle used for isolated configuration setup and assertions"
func Test_ReadEmptyV1(t *testing.T) {
	setup(t, "ReadEmpty")

	c, err := ReadDaemonConfig()
	assert.NilError(t, err)
	assert.Assert(t, len(c.Repos) == 0)
}

// @description    Verifies the daemon configuration modification time.
//
// Test_DaemonConfigModTime verifies that the modification time is the zero time when no
// configuration file exists and a non-zero time after a configuration is written.
//
// @param           t   "test handle used for isolated configuration setup and assertions"
func Test_DaemonConfigModTime(t *testing.T) {
	setup(t, "ModTime")

	mod, err := DaemonConfigModTime()
	assert.NilError(t, err)
	assert.Assert(t, mod.IsZero(), "expected zero mod time when config file is absent")

	err = WriteDaemonConfig(&DaemonConfig{Repos: []string{"/repo"}})
	assert.NilError(t, err)

	mod, err = DaemonConfigModTime()
	assert.NilError(t, err)
	assert.Assert(t, !mod.IsZero(), "expected non-zero mod time after writing config")
}
