package daemonstate

import (
	"testing"
	"time"
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state == nil {
		t.Fatalf("assertion failed: state != nil")
	}
	if len(state.Repos) != 0 {
		t.Fatalf("assertion failed: len(state.Repos) == 0")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mod.IsZero() {
		t.Fatalf("assertion failed: mod.IsZero()")
	}

	if err := WriteState(&State{Repos: []RepoStatus{{Repo: "/repo", Status: StatusRunning}}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mod, err = StateModTime()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mod.IsZero() {
		t.Fatalf("assertion failed: !mod.IsZero()")
	}
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
	if err := WriteState(want); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Repos) != len(want.Repos) {
		t.Fatalf("got %v, want %v", len(got.Repos), len(want.Repos))
	}
	for i := range want.Repos {
		gotRepo, wantRepo := got.Repos[i], want.Repos[i]
		// Compare instants with Time.Equal, not reflect.DeepEqual: read-back times round-trip
		// through state.json and carry the UTC location, while want carries time.Local, and
		// DeepEqual compares the Location pointer, which flakes under TZ=UTC.
		if gotRepo.Repo != wantRepo.Repo || gotRepo.Status != wantRepo.Status || gotRepo.Stage != wantRepo.Stage ||
			!gotRepo.UpdatedAt.Equal(wantRepo.UpdatedAt) || !gotRepo.LastSyncedAt.Equal(wantRepo.LastSyncedAt) {
			t.Fatalf("got %#v, want %#v", gotRepo, wantRepo)
		}
	}
}

// @description    Verifies WriteState sorts repositories by path.
//
// Test_WriteStateSortsRepos writes a state with repositories out of order and verifies the persisted
// file lists them sorted by path, so output is stable across writes.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_WriteStateSortsRepos(t *testing.T) {
	setupState(t)

	if err := WriteState(&State{Repos: []RepoStatus{
		{Repo: "/z", Status: StatusRunning},
		{Repo: "/a", Status: StatusRunning},
		{Repo: "/m", Status: StatusRunning},
	}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Repos) != 3 {
		t.Fatalf("got %v, want %v", len(got.Repos), 3)
	}
	if got.Repos[0].Repo != "/a" {
		t.Fatalf("got %v, want %v", got.Repos[0].Repo, "/a")
	}
	if got.Repos[1].Repo != "/m" {
		t.Fatalf("got %v, want %v", got.Repos[1].Repo, "/m")
	}
	if got.Repos[2].Repo != "/z" {
		t.Fatalf("got %v, want %v", got.Repos[2].Repo, "/z")
	}
}

// @description    Verifies WriteState overwrites existing content.
//
// Test_WriteStateOverwrites writes one repository, then writes a different set, and verifies the
// second write fully replaced the first.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_WriteStateOverwrites(t *testing.T) {
	setupState(t)

	if err := WriteState(&State{Repos: []RepoStatus{
		{Repo: "/old", Status: StatusRunning},
	}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := WriteState(&State{Repos: []RepoStatus{
		{Repo: "/new", Status: StatusPaused, Stage: "author"},
	}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Repos) != 1 {
		t.Fatalf("got %v, want %v", len(got.Repos), 1)
	}
	if got.Repos[0].Repo != "/new" {
		t.Fatalf("got %v, want %v", got.Repos[0].Repo, "/new")
	}
	if got.Repos[0].Stage != "author" {
		t.Fatalf("got %v, want %v", got.Repos[0].Stage, "author")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(first.Repos) != 1 {
		t.Fatalf("got %v, want %v", len(first.Repos), 1)
	}
	if first.Repos[0].Status != StatusRunning {
		t.Fatalf("got %v, want %v", first.Repos[0].Status, StatusRunning)
	}
	firstUpdated := first.Repos[0].UpdatedAt

	// An identical report must not refresh UpdatedAt.
	time.Sleep(10 * time.Millisecond)
	r.Set("/repo", StatusRunning, "", time.Time{})
	second, err := ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.Repos[0].UpdatedAt != firstUpdated {
		t.Fatalf("got %v, want %v", second.Repos[0].UpdatedAt, firstUpdated)
	}

	// A changed status updates the entry and refreshes UpdatedAt.
	r.Set("/repo", StatusPaused, "rebase", time.Time{})
	third, err := ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if third.Repos[0].Status != StatusPaused {
		t.Fatalf("got %v, want %v", third.Repos[0].Status, StatusPaused)
	}
	if third.Repos[0].Stage != "rebase" {
		t.Fatalf("got %v, want %v", third.Repos[0].Stage, "rebase")
	}
	if !third.Repos[0].UpdatedAt.After(firstUpdated) {
		t.Fatalf("assertion failed: third.Repos[0].UpdatedAt.After(firstUpdated)")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mod.IsZero() {
		t.Fatalf("assertion failed: mod.IsZero()")
	}

	r.Set("/repo", StatusRunning, "", time.Time{})
	before, err := ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	r.Heartbeat()
	after, err := ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !after.Repos[0].UpdatedAt.After(before.Repos[0].UpdatedAt) {
		t.Fatalf("assertion failed: after.Repos[0].UpdatedAt.After(before.Repos[0].UpdatedAt)")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Repos) != 1 {
		t.Fatalf("got %v, want %v", len(got.Repos), 1)
	}
	if got.Repos[0].Repo != "/repo/b" {
		t.Fatalf("got %v, want %v", got.Repos[0].Repo, "/repo/b")
	}

	// Removing an unknown repository is a no-op.
	r.Remove("/repo/missing")
	got, err = ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Repos) != 1 {
		t.Fatalf("got %v, want %v", len(got.Repos), 1)
	}
}

// @description    Verifies Recorder.Remove deletes state not loaded into runtime memory.
//
// Test_RecorderRemovePersistedOnly seeds state.json before creating the recorder, then removes the
// repository without any watcher report. The persisted entry must be deleted even though the
// recorder has no in-memory runtime state for it.
//
// @param           t  "test handle used for isolated setup and assertions"
func Test_RecorderRemovePersistedOnly(t *testing.T) {
	setupState(t)
	if err := WriteState(&State{Repos: []RepoStatus{
		{Repo: "/repo", Status: StatusRunning, LastSyncedAt: time.Unix(5000, 0)},
		{Repo: "/other", Status: StatusPaused, Stage: "commit", LastSyncedAt: time.Unix(4000, 0)},
	}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := NewRecorder()
	r.Remove("/repo")

	got, err := ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Repos) != 1 {
		t.Fatalf("got %v, want %v", len(got.Repos), 1)
	}
	if got.Repos[0].Repo != "/other" {
		t.Fatalf("got %v, want %v", got.Repos[0].Repo, "/other")
	}
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Compare instants with Time.Equal, not ==: got.LastSyncedAt round-trips through state.json
	// and carries the UTC location, while first carries time.Local, and == compares the Location
	// pointer, which flakes under TZ=UTC.
	if !got.Repos[0].LastSyncedAt.Equal(first) {
		t.Fatalf("assertion failed: got.Repos[0].LastSyncedAt.Equal(first)")
	}

	// A newer LastSyncedAt with identical status and stage writes.
	second := time.Unix(2000, 0)
	r.Set("/repo", StatusRunning, "", second)
	got, err = ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Repos[0].LastSyncedAt.Equal(second) {
		t.Fatalf("assertion failed: got.Repos[0].LastSyncedAt.Equal(second)")
	}

	// The same LastSyncedAt is a no-op: LastSyncedAt is unchanged and the on-disk value stays second.
	r.Set("/repo", StatusRunning, "", second)
	got, err = ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Repos[0].LastSyncedAt.Equal(second) {
		t.Fatalf("assertion failed: got.Repos[0].LastSyncedAt.Equal(second)")
	}
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
	if err := WriteState(&State{Repos: []RepoStatus{
		{Repo: "/repo", Status: StatusRunning, UpdatedAt: time.Unix(1500, 0), LastSyncedAt: time.Unix(5000, 0)},
	}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The daemon persists again (e.g. heartbeat-driven Set with zero lastSynced). The newer on-disk
	// LastSyncedAt must survive.
	r.Set("/repo", StatusRunning, "", time.Time{})
	got, err := ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Repos[0].LastSyncedAt.Unix() != int64(5000) {
		t.Fatalf("got %v, want %v", got.Repos[0].LastSyncedAt.Unix(), int64(5000))
	}
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
		if err := WriteState(&State{Repos: []RepoStatus{
			{Repo: "/repo", Status: StatusPaused, Stage: "rebase", UpdatedAt: time.Unix(1000, 0)},
		}}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := RecordSyncSuccess("/repo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := ReadState()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Repos) != 1 {
			t.Fatalf("got %v, want %v", len(got.Repos), 1)
		}
		if got.Repos[0].Status != StatusPaused {
			t.Fatalf("got %v, want %v", got.Repos[0].Status, StatusPaused)
		}
		if got.Repos[0].Stage != "rebase" {
			t.Fatalf("got %v, want %v", got.Repos[0].Stage, "rebase")
		}
		if got.Repos[0].UpdatedAt.Unix() != int64(1000) {
			t.Fatalf("got %v, want %v", got.Repos[0].UpdatedAt.Unix(), int64(1000))
		}
		if got.Repos[0].LastSyncedAt.IsZero() {
			t.Fatalf("assertion failed: !got.Repos[0].LastSyncedAt.IsZero()")
		}
	})

	t.Run("missing entry creates running entry", func(t *testing.T) {
		setupState(t)
		if err := WriteState(&State{Repos: []RepoStatus{
			{Repo: "/other", Status: StatusRunning, UpdatedAt: time.Unix(1000, 0)},
		}}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if err := RecordSyncSuccess("/repo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := ReadState()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Repos) != 2 {
			t.Fatalf("got %v, want %v", len(got.Repos), 2)
		}
		if got.Repos[1].Repo != "/repo" {
			t.Fatalf("got %v, want %v", got.Repos[1].Repo, "/repo")
		}
		if got.Repos[1].Status != StatusRunning {
			t.Fatalf("got %v, want %v", got.Repos[1].Status, StatusRunning)
		}
		if got.Repos[1].LastSyncedAt.IsZero() {
			t.Fatalf("assertion failed: !got.Repos[1].LastSyncedAt.IsZero()")
		}
	})

	t.Run("no state file", func(t *testing.T) {
		setupState(t)
		if err := RecordSyncSuccess("/repo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := ReadState()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Repos) != 1 {
			t.Fatalf("got %v, want %v", len(got.Repos), 1)
		}
		if got.Repos[0].LastSyncedAt.IsZero() {
			t.Fatalf("assertion failed: !got.Repos[0].LastSyncedAt.IsZero()")
		}
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

	if (RepoStatus{UpdatedAt: now}).IsStale(now) {
		t.Fatalf("assertion failed: !RepoStatus{UpdatedAt: now}.IsStale(now)")
	}
	if !(RepoStatus{UpdatedAt: now.Add(-(StaleThreshold + time.Second))}).IsStale(now) {
		t.Fatalf("assertion failed: RepoStatus{UpdatedAt: now.Add(-(StaleThreshold + time.Second))}.IsStale(now)")
	}
	if !(RepoStatus{}).IsStale(now) {
		t.Fatalf("assertion failed: RepoStatus{}.IsStale(now)")
	}
}
