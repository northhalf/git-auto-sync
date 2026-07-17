package daemonstate

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

// setupState points XDG_CONFIG_HOME and HOME at a temporary directory so StateFile, ReadState, and
// WriteState target an isolated state.json.
func setupState(t *testing.T) {
	t.Helper()
	newConfigDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", newConfigDir)
	t.Setenv("HOME", newConfigDir)
}

// @description    Verifies reading without a state file.
//
// Test_StateReadEmpty verifies that reading from an isolated configuration directory with no state
// file succeeds and returns an empty state.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_StateReadEmpty(t *testing.T) {
	setupState(t)

	state, err := ReadState()
	assert.NilError(t, err)
	assert.Assert(t, state != nil)
	assert.Assert(t, len(state.Repos) == 0)
}

// @description    Verifies the state file modification time.
//
// Test_StateModTime verifies that the modification time is the zero time when no state file exists
// and a non-zero time after state is written.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_StateModTime(t *testing.T) {
	setupState(t)

	mod, err := StateModTime()
	assert.NilError(t, err)
	assert.Assert(t, mod.IsZero(), "expected zero mod time when state file is absent")

	assert.NilError(t, WriteState(&State{Repos: []RepoStatus{{Repo: "/repo", Status: StatusRunning}}}))

	mod, err = StateModTime()
	assert.NilError(t, err)
	assert.Assert(t, !mod.IsZero(), "expected non-zero mod time after writing state")
}

// @description    Verifies state round-trips through the file.
//
// Test_StateRoundTrip writes a state with two repositories and reads back an equal value.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_StateRoundTrip(t *testing.T) {
	setupState(t)

	want := &State{Repos: []RepoStatus{
		{Repo: "/repo/a", Status: StatusRunning, UpdatedAt: time.Unix(1000, 0)},
		{Repo: "/repo/b", Status: StatusPaused, Stage: "rebase", UpdatedAt: time.Unix(2000, 0)},
	}}
	assert.NilError(t, WriteState(want))

	got, err := ReadState()
	assert.NilError(t, err)
	assert.DeepEqual(t, got, want)
}

// @description    Verifies WriteState sorts repositories by path.
//
// Test_WriteStateSortsRepos writes a state with repositories out of order and verifies the persisted
// file lists them sorted by path, so output is stable across writes.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_WriteStateSortsRepos(t *testing.T) {
	setupState(t)

	assert.NilError(t, WriteState(&State{Repos: []RepoStatus{
		{Repo: "/z", Status: StatusRunning},
		{Repo: "/a", Status: StatusRunning},
		{Repo: "/m", Status: StatusRunning},
	}}))

	got, err := ReadState()
	assert.NilError(t, err)
	assert.Equal(t, len(got.Repos), 3)
	assert.Equal(t, got.Repos[0].Repo, "/a")
	assert.Equal(t, got.Repos[1].Repo, "/m")
	assert.Equal(t, got.Repos[2].Repo, "/z")
}

// @description    Verifies WriteState overwrites existing content.
//
// Test_WriteStateOverwrites writes one repository, then writes a different set, and verifies the
// second write fully replaced the first.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_WriteStateOverwrites(t *testing.T) {
	setupState(t)

	assert.NilError(t, WriteState(&State{Repos: []RepoStatus{
		{Repo: "/old", Status: StatusRunning},
	}}))
	assert.NilError(t, WriteState(&State{Repos: []RepoStatus{
		{Repo: "/new", Status: StatusPaused, Stage: "author"},
	}}))

	got, err := ReadState()
	assert.NilError(t, err)
	assert.Equal(t, len(got.Repos), 1)
	assert.Equal(t, got.Repos[0].Repo, "/new")
	assert.Equal(t, got.Repos[0].Stage, "author")
}

// @description    Verifies Recorder.Set persists and deduplicates.
//
// Test_RecorderSetPersistsAndDedupes verifies that the first Set writes state.json, that a second
// identical Set is a no-op (UpdatedAt unchanged), and that a Set with a changed status updates the
// entry.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_RecorderSetPersistsAndDedupes(t *testing.T) {
	setupState(t)
	r := NewRecorder()

	r.Set("/repo", StatusRunning, "")
	first, err := ReadState()
	assert.NilError(t, err)
	assert.Equal(t, len(first.Repos), 1)
	assert.Equal(t, first.Repos[0].Status, StatusRunning)
	firstUpdated := first.Repos[0].UpdatedAt

	// An identical report must not refresh UpdatedAt.
	time.Sleep(10 * time.Millisecond)
	r.Set("/repo", StatusRunning, "")
	second, err := ReadState()
	assert.NilError(t, err)
	assert.Equal(t, second.Repos[0].UpdatedAt, firstUpdated)

	// A changed status updates the entry and refreshes UpdatedAt.
	r.Set("/repo", StatusPaused, "rebase")
	third, err := ReadState()
	assert.NilError(t, err)
	assert.Equal(t, third.Repos[0].Status, StatusPaused)
	assert.Equal(t, third.Repos[0].Stage, "rebase")
	assert.Assert(t, third.Repos[0].UpdatedAt.After(firstUpdated))
}

// @description    Verifies Recorder.Heartbeat refreshes every entry.
//
// Test_RecorderHeartbeat verifies that Heartbeat bumps UpdatedAt past a previously recorded value,
// and that it is a no-op (and writes no file) when no repositories are tracked.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_RecorderHeartbeat(t *testing.T) {
	setupState(t)

	// No repositories: Heartbeat must not create state.json.
	r := NewRecorder()
	r.Heartbeat()
	mod, err := StateModTime()
	assert.NilError(t, err)
	assert.Assert(t, mod.IsZero(), "Heartbeat with no repos should not write a state file")

	r.Set("/repo", StatusRunning, "")
	before, err := ReadState()
	assert.NilError(t, err)

	time.Sleep(10 * time.Millisecond)
	r.Heartbeat()
	after, err := ReadState()
	assert.NilError(t, err)
	assert.Assert(t, after.Repos[0].UpdatedAt.After(before.Repos[0].UpdatedAt))
}

// @description    Verifies Recorder.Remove deletes an entry.
//
// Test_RecorderRemove verifies that Remove deletes the named repository and is a no-op for an
// unknown repository.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_RecorderRemove(t *testing.T) {
	setupState(t)
	r := NewRecorder()
	r.Set("/repo/a", StatusRunning, "")
	r.Set("/repo/b", StatusPaused, "commit")

	r.Remove("/repo/a")
	got, err := ReadState()
	assert.NilError(t, err)
	assert.Equal(t, len(got.Repos), 1)
	assert.Equal(t, got.Repos[0].Repo, "/repo/b")

	// Removing an unknown repository is a no-op.
	r.Remove("/repo/missing")
	got, err = ReadState()
	assert.NilError(t, err)
	assert.Equal(t, len(got.Repos), 1)
}

// @description    Verifies staleness detection.
//
// Test_RepoStatusIsStale verifies that a freshly updated entry is not stale, an entry older than
// StaleThreshold is stale, and an entry with a zero UpdatedAt is stale.
//
// @param           t  "test handle used for assertions"
func Test_RepoStatusIsStale(t *testing.T) {
	now := time.Unix(10_000, 0)

	assert.Assert(t, !RepoStatus{UpdatedAt: now}.IsStale(now), "current entry should not be stale")
	assert.Assert(t, RepoStatus{UpdatedAt: now.Add(-(StaleThreshold + time.Second))}.IsStale(now), "old entry should be stale")
	assert.Assert(t, RepoStatus{}.IsStale(now), "zero UpdatedAt should be stale")
}
