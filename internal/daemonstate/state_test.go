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
		{Repo: "/repo/a", Status: StatusRunning, UpdatedAt: time.Unix(1000, 0), LastSyncedAt: time.Unix(1500, 0)},
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

	r.Set("/repo", StatusRunning, "", time.Time{})
	first, err := ReadState()
	assert.NilError(t, err)
	assert.Equal(t, len(first.Repos), 1)
	assert.Equal(t, first.Repos[0].Status, StatusRunning)
	firstUpdated := first.Repos[0].UpdatedAt

	// An identical report must not refresh UpdatedAt.
	time.Sleep(10 * time.Millisecond)
	r.Set("/repo", StatusRunning, "", time.Time{})
	second, err := ReadState()
	assert.NilError(t, err)
	assert.Equal(t, second.Repos[0].UpdatedAt, firstUpdated)

	// A changed status updates the entry and refreshes UpdatedAt.
	r.Set("/repo", StatusPaused, "rebase", time.Time{})
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

	r.Set("/repo", StatusRunning, "", time.Time{})
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
	r.Set("/repo/a", StatusRunning, "", time.Time{})
	r.Set("/repo/b", StatusPaused, "commit", time.Time{})

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

// @description    Verifies Recorder.Set dedup covers LastSyncedAt.
//
// Test_RecorderSetDedupesLastSynced verifies that a Set carrying a new LastSyncedAt writes state.json
// even when the status and stage are unchanged, and that a Set with the same LastSyncedAt as the
// current entry is a no-op.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_RecorderSetDedupesLastSynced(t *testing.T) {
	setupState(t)
	r := NewRecorder()

	first := time.Unix(1000, 0)
	r.Set("/repo", StatusRunning, "", first)
	got, err := ReadState()
	assert.NilError(t, err)
	// Compare instants with Time.Equal, not assert.Equal: got.LastSyncedAt round-trips through
	// state.json and carries the UTC location, while first carries time.Local, and assert.Equal
	// uses == (Location pointer comparison), which flakes under TZ=UTC.
	assert.Assert(t, got.Repos[0].LastSyncedAt.Equal(first))

	// A newer LastSyncedAt with identical status and stage writes.
	second := time.Unix(2000, 0)
	r.Set("/repo", StatusRunning, "", second)
	got, err = ReadState()
	assert.NilError(t, err)
	assert.Assert(t, got.Repos[0].LastSyncedAt.Equal(second))

	// The same LastSyncedAt is a no-op: LastSyncedAt is unchanged and the on-disk value stays second.
	r.Set("/repo", StatusRunning, "", second)
	got, err = ReadState()
	assert.NilError(t, err)
	assert.Assert(t, got.Repos[0].LastSyncedAt.Equal(second))
}

// @description    Verifies persistLocked preserves a newer on-disk LastSyncedAt.
//
// Test_RecorderPreservesDiskLastSynced writes a state.json whose LastSyncedAt is newer than the
// recorder's in-memory value, then triggers a persist through Set. The merge in persistLocked must
// keep the newer on-disk timestamp, simulating a CLI sync that landed between daemon writes.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_RecorderPreservesDiskLastSynced(t *testing.T) {
	setupState(t)

	// Seed the recorder with an older LastSyncedAt.
	r := NewRecorder()
	r.Set("/repo", StatusRunning, "", time.Unix(1000, 0))

	// A separate process (the CLI) writes a newer LastSyncedAt directly to state.json.
	assert.NilError(t, WriteState(&State{Repos: []RepoStatus{
		{Repo: "/repo", Status: StatusRunning, UpdatedAt: time.Unix(1500, 0), LastSyncedAt: time.Unix(5000, 0)},
	}}))

	// The daemon persists again (e.g. heartbeat-driven Set with zero lastSynced). The newer on-disk
	// LastSyncedAt must survive.
	r.Set("/repo", StatusRunning, "", time.Time{})
	got, err := ReadState()
	assert.NilError(t, err)
	assert.Equal(t, got.Repos[0].LastSyncedAt.Unix(), int64(5000))
}

// @description    Verifies RecordSyncSuccess merges LastSyncedAt into state.json.
//
// Test_RecordSyncSuccess covers the three cases the CLI sync command exercises: an existing entry
// keeps its status, stage, and UpdatedAt and gains a fresh LastSyncedAt; a missing entry creates a
// running entry; and the call works when no state file exists yet.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_RecordSyncSuccess(t *testing.T) {
	t.Run("existing entry preserves runtime fields", func(t *testing.T) {
		setupState(t)
		assert.NilError(t, WriteState(&State{Repos: []RepoStatus{
			{Repo: "/repo", Status: StatusPaused, Stage: "rebase", UpdatedAt: time.Unix(1000, 0)},
		}}))

		assert.NilError(t, RecordSyncSuccess("/repo"))
		got, err := ReadState()
		assert.NilError(t, err)
		assert.Equal(t, len(got.Repos), 1)
		assert.Equal(t, got.Repos[0].Status, StatusPaused)
		assert.Equal(t, got.Repos[0].Stage, "rebase")
		assert.Equal(t, got.Repos[0].UpdatedAt.Unix(), int64(1000))
		assert.Assert(t, !got.Repos[0].LastSyncedAt.IsZero(), "LastSyncedAt should be set")
	})

	t.Run("missing entry creates running entry", func(t *testing.T) {
		setupState(t)
		assert.NilError(t, WriteState(&State{Repos: []RepoStatus{
			{Repo: "/other", Status: StatusRunning, UpdatedAt: time.Unix(1000, 0)},
		}}))

		assert.NilError(t, RecordSyncSuccess("/repo"))
		got, err := ReadState()
		assert.NilError(t, err)
		assert.Equal(t, len(got.Repos), 2)
		assert.Equal(t, got.Repos[1].Repo, "/repo")
		assert.Equal(t, got.Repos[1].Status, StatusRunning)
		assert.Assert(t, !got.Repos[1].LastSyncedAt.IsZero(), "LastSyncedAt should be set")
	})

	t.Run("no state file", func(t *testing.T) {
		setupState(t)
		assert.NilError(t, RecordSyncSuccess("/repo"))
		got, err := ReadState()
		assert.NilError(t, err)
		assert.Equal(t, len(got.Repos), 1)
		assert.Assert(t, !got.Repos[0].LastSyncedAt.IsZero(), "LastSyncedAt should be set")
	})
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
