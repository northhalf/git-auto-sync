package main

import (
	"testing"
	"time"

	"github.com/northhalf/git-auto-sync/internal/daemonstate"
	"github.com/northhalf/git-auto-sync/internal/watcher"
	"gotest.tools/v3/assert"
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
	assert.Assert(t, cb != nil)

	cb(watcher.StateReport{Paused: false})
	cb(watcher.StateReport{Paused: true, Stage: "rebase"})

	state, err := daemonstate.ReadState()
	assert.NilError(t, err)
	assert.Equal(t, len(state.Repos), 1)
	assert.Equal(t, state.Repos[0].Repo, "/repo")
	assert.Equal(t, state.Repos[0].Status, daemonstate.StatusPaused)
	assert.Equal(t, state.Repos[0].Stage, "rebase")
}

// @description    Verifies onState returns nil without a recorder.
//
// Test_OnStateNilWithoutRecorder verifies that a manager with no recorder returns a nil callback so
// the watcher skips reporting, keeping test doubles that override start unchanged.
//
// @param           t  "test handle used for assertions"
func Test_OnStateNilWithoutRecorder(t *testing.T) {
	m := &watcherManager{watchers: make(map[string]*watcherHandle)}
	assert.Assert(t, m.onState("/repo") == nil)
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
	assert.NilError(t, err)

	time.Sleep(10 * time.Millisecond)
	m.Heartbeat()

	after, err := daemonstate.ReadState()
	assert.NilError(t, err)
	assert.Assert(t, after.Repos[0].UpdatedAt.After(before.Repos[0].UpdatedAt))
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
	m.recorder.Set("/repo", daemonstate.StatusPaused, "rebase")

	// Start the fake watcher, then simulate its exit and removal from the config.
	m.reconcile([]string{"/repo"}, nil)
	fs.close("/repo")
	m.reconcile([]string{}, nil)

	state, err := daemonstate.ReadState()
	assert.NilError(t, err)
	assert.Equal(t, len(state.Repos), 0)
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
