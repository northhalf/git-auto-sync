package main

import (
	"testing"
	"time"

	"github.com/northhalf/git-auto-sync/internal/daemonstate"
	"github.com/northhalf/git-auto-sync/internal/watcher"
)

// setupDaemonState points XDG_CONFIG_HOME and HOME at a temporary directory so the manager's recorder
// writes state.json to an isolated location.
func setupDaemonState(t *testing.T) {
	t.Helper()
	newConfigDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", newConfigDir)
	t.Setenv("HOME", newConfigDir)
}

// @description    Verifies onState records watcher transitions.
//
// Test_OnStateRecordsTransitions verifies that the callback built by onState records a running
// report and then a paused report with the failed stage, persisting the latest status to state.json.
//
// @param           t  "test handle used for isolated state setup and assertions"
func Test_OnStateRecordsTransitions(t *testing.T) {
	setupDaemonState(t)
	m := newWatcherManager()

	cb := m.onState("/repo")
	if cb == nil {
		t.Fatalf("assertion failed: cb != nil")
	}

	cb(watcher.StateReport{Paused: false})
	cb(watcher.StateReport{Paused: true, Stage: "rebase"})

	state, err := daemonstate.ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Repos) != 1 {
		t.Fatalf("got %v, want %v", len(state.Repos), 1)
	}
	if state.Repos[0].Repo != "/repo" {
		t.Fatalf("got %v, want %v", state.Repos[0].Repo, "/repo")
	}
	if state.Repos[0].Status != daemonstate.StatusPaused {
		t.Fatalf("got %v, want %v", state.Repos[0].Status, daemonstate.StatusPaused)
	}
	if state.Repos[0].Stage != "rebase" {
		t.Fatalf("got %v, want %v", state.Repos[0].Stage, "rebase")
	}
}

// @description    Verifies onState returns nil without a recorder.
//
// Test_OnStateNilWithoutRecorder verifies that a manager with no recorder returns a nil callback so
// the watcher skips reporting, keeping test doubles that override start unchanged.
//
// @param           t  "test handle used for assertions"
func Test_OnStateNilWithoutRecorder(t *testing.T) {
	m := &watcherManager{watchers: make(map[string]*watcherHandle)}
	if m.onState("/repo") != nil {
		t.Fatalf("assertion failed: m.onState(\"/repo\") == nil")
	}
}

// @description    Verifies Heartbeat refreshes persisted timestamps.
//
// Test_HeartbeatRefreshesState verifies that Heartbeat bumps a repository's UpdatedAt past the value
// recorded by an earlier onState report.
//
// @param           t  "test handle used for isolated state setup and assertions"
func Test_HeartbeatRefreshesState(t *testing.T) {
	setupDaemonState(t)
	m := newWatcherManager()

	m.onState("/repo")(watcher.StateReport{Paused: false})
	before, err := daemonstate.ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	m.Heartbeat()

	after, err := daemonstate.ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !after.Repos[0].UpdatedAt.After(before.Repos[0].UpdatedAt) {
		t.Fatalf("assertion failed: after.Repos[0].UpdatedAt.After(before.Repos[0].UpdatedAt)")
	}
}

// @description    Verifies reconcile removes state when a watcher exits.
//
// Test_ReconcileRemovesStateOnExit verifies that a recorded repository whose watcher exits is
// removed from state.json on the next reconcile, so the CLI does not report a stale entry for a
// repository that is no longer monitored.
//
// @param           t  "test handle used for isolated state setup and assertions"
func Test_ReconcileRemovesStateOnExit(t *testing.T) {
	setupDaemonState(t)
	fs := newFakeStart()
	m := &watcherManager{
		watchers: make(map[string]*watcherHandle),
		start:    fs.start,
		recorder: daemonstate.NewRecorder(),
	}
	m.recorder.Set("/repo", daemonstate.StatusPaused, "rebase", time.Time{})

	// Start the fake watcher, then simulate its exit and removal from the config.
	m.reconcile([]string{"/repo"}, nil)
	fs.close("/repo")
	m.reconcile([]string{}, nil)

	state, err := daemonstate.ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Repos) != 0 {
		t.Fatalf("got %v, want %v", len(state.Repos), 0)
	}
}

// @description    Verifies an unexpected watcher restart preserves its persisted state.
//
// Test_ReconcilePreservesStateOnWatcherRestart records a successful sync, lets the watcher exit while
// the repository remains configured, and reconciles a replacement watcher. The last sync must remain
// on disk, and Heartbeat must not refresh the forgotten runtime entry before the replacement reports.
//
// @param           t  "test handle used for isolated state setup and assertions"
func Test_ReconcilePreservesStateOnWatcherRestart(t *testing.T) {
	setupDaemonState(t)
	fs := newFakeStart()
	m := &watcherManager{
		watchers: make(map[string]*watcherHandle),
		start:    fs.start,
		recorder: daemonstate.NewRecorder(),
	}
	lastSynced := time.Unix(5000, 0)
	m.recorder.Set("/repo", daemonstate.StatusRunning, "", lastSynced)

	m.reconcile([]string{"/repo"}, nil)
	fs.close("/repo")
	m.reconcile([]string{"/repo"}, nil)

	state, err := daemonstate.ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Repos) != 1 {
		t.Fatalf("got %v, want %v", len(state.Repos), 1)
	}
	if !state.Repos[0].LastSyncedAt.Equal(lastSynced) {
		t.Fatalf("assertion failed: state.Repos[0].LastSyncedAt.Equal(lastSynced)")
	}
	updatedAt := state.Repos[0].UpdatedAt
	if len(fs.started) != 2 {
		t.Fatalf("got %v, want %v", len(fs.started), 2)
	}

	time.Sleep(10 * time.Millisecond)
	m.Heartbeat()
	state, err = daemonstate.ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !state.Repos[0].UpdatedAt.Equal(updatedAt) {
		t.Fatalf("assertion failed: state.Repos[0].UpdatedAt.Equal(updatedAt)")
	}
}

// @description    Verifies configuration removal deletes state preserved across a restart.
//
// Test_ReconcileRemovesForgottenStateAfterConfigRemoval first restarts an exited watcher while its
// repository remains configured, which forgets runtime state but preserves the state file. Removing
// the repository after the replacement exits must still delete the persisted entry.
//
// @param           t  "test handle used for isolated state setup and assertions"
func Test_ReconcileRemovesForgottenStateAfterConfigRemoval(t *testing.T) {
	setupDaemonState(t)
	fs := newFakeStart()
	m := &watcherManager{
		watchers: make(map[string]*watcherHandle),
		start:    fs.start,
		recorder: daemonstate.NewRecorder(),
	}
	m.recorder.Set("/repo", daemonstate.StatusRunning, "", time.Unix(5000, 0))

	m.reconcile([]string{"/repo"}, nil)
	fs.close("/repo")
	m.reconcile([]string{"/repo"}, nil)
	fs.close("/repo")
	m.reconcile(nil, nil)

	state, err := daemonstate.ReadState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Repos) != 0 {
		t.Fatalf("got %v, want %v", len(state.Repos), 0)
	}
}

// @description    Verifies Heartbeat is a no-op without a recorder.
//
// Test_HeartbeatNoOpWithoutRecorder verifies that Heartbeat on a manager with no recorder does not
// panic and writes no state file.
//
// @param           t  "test handle used for assertions"
func Test_HeartbeatNoOpWithoutRecorder(t *testing.T) {
	m := &watcherManager{watchers: make(map[string]*watcherHandle)}
	m.Heartbeat()
}
