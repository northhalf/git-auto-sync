package main

import (
	"context"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/logging"
	"gotest.tools/v3/assert"
)

// fakeStartCall records one invocation of the manager's start function.
type fakeStartCall struct {
	repo string
	envs []string
}

// fakeStart is a test double for the manager's start function. It records every repository it is
// asked to start, returns a handle whose done channel stays open until close is called for that
// repository, and never starts a real watcher.
type fakeStart struct {
	started []fakeStartCall
	handles map[string]chan struct{}
}

// @description    Creates a fake start function for watcher manager tests.
//
// newFakeStart returns a fakeStart that records start calls and never starts a real watcher.
//
// @return          *fakeStart   "initialized fake start double"
func newFakeStart() *fakeStart {
	return &fakeStart{handles: make(map[string]chan struct{})}
}

// @description    Records a start request and returns a non-real handle.
//
// start appends the call to the fake's history and returns a handle whose done channel stays open
// until close is called for that repository.
//
// @param           repo            "repository path passed to start"
//
// @param           envs            "environment entries passed to start"
//
// @return          *watcherHandle  "handle backed by an open done channel"
func (f *fakeStart) start(repo string, envs []string) *watcherHandle {
	f.started = append(f.started, fakeStartCall{repo: repo, envs: append([]string(nil), envs...)})
	done := make(chan struct{})
	f.handles[repo] = done
	return &watcherHandle{done: done}
}

// @description    Simulates a watcher exiting for the given repository.
//
// close closes the done channel recorded for repo, which reconcile treats as an exited watcher.
//
// @param           repo   "repository path whose watcher should exit"
func (f *fakeStart) close(repo string) {
	if done, ok := f.handles[repo]; ok {
		close(done)
	}
}

// @description    Verifies watcherManager reconciliation.
//
// Test_WatcherManagerReconcile verifies that reconcile starts a watcher for each configured
// repository, does not duplicate running watchers, cleans up exited watchers, and starts a fresh
// watcher for a repository that is re-added after its previous watcher exited.
//
// @param           t   "test handle used for assertions"
func Test_WatcherManagerReconcile(t *testing.T) {
	fs := newFakeStart()
	mgr := &watcherManager{watchers: make(map[string]*watcherHandle), start: fs.start}

	// Initial reconcile starts /a and /b.
	mgr.reconcile([]string{"/a", "/b"}, []string{"K=V"})
	assert.Equal(t, len(fs.started), 2)
	assert.Equal(t, fs.started[0].repo, "/a")
	assert.Equal(t, fs.started[1].repo, "/b")
	assert.DeepEqual(t, fs.started[0].envs, []string{"K=V"})

	// Reconciling the same set starts nothing new.
	mgr.reconcile([]string{"/a", "/b"}, []string{"K=V"})
	assert.Equal(t, len(fs.started), 2)

	// /b is removed from the config and its watcher exits: reconcile drops it without restarting.
	fs.close("/b")
	mgr.reconcile([]string{"/a"}, []string{"K=V"})
	assert.Equal(t, len(fs.started), 2)

	// Re-adding /b starts a fresh watcher.
	mgr.reconcile([]string{"/a", "/b"}, []string{"K=V"})
	assert.Equal(t, len(fs.started), 3)
	assert.Equal(t, fs.started[2].repo, "/b")

	// /a removed from the config but still running: not cleaned up and not restarted.
	mgr.reconcile([]string{}, []string{"K=V"})
	assert.Equal(t, len(fs.started), 3)

	// /a's watcher now exits: reconcile cleans it up; nothing is started.
	fs.close("/a")
	mgr.reconcile([]string{}, []string{"K=V"})
	assert.Equal(t, len(fs.started), 3)
}

// @description    Verifies settingsChanged detects field changes.
//
// Test_SettingsChanged verifies that equal settings report unchanged, and any one changed field
// reports changed.
//
// @param           t   "test handle used for assertions"
func Test_SettingsChanged(t *testing.T) {
	a := 60
	b := 10
	gitA := "/usr/bin/git"
	base := &config.Settings{SyncInterval: &a, Debounce: &b, GitExec: &gitA}
	assert.Assert(t, !settingsChanged(base, &config.Settings{SyncInterval: &a, Debounce: &b, GitExec: &gitA}))
	assert.Assert(t, settingsChanged(base, &config.Settings{SyncInterval: ptrInt(120), Debounce: &b, GitExec: &gitA}))
	assert.Assert(t, settingsChanged(base, &config.Settings{SyncInterval: &a, Debounce: ptrInt(20), GitExec: &gitA}))
	assert.Assert(t, settingsChanged(base, &config.Settings{SyncInterval: &a, Debounce: &b, GitExec: ptrStr("/opt/git")}))
	assert.Assert(t, !settingsChanged(nil, nil))
	assert.Assert(t, settingsChanged(base, nil))
}

// @description    Verifies RestartAll cancels every handle and clears the map.
//
// Test_RestartAll verifies that after RestartAll every started handle is canceled and the manager
// map is empty, so a subsequent reconcile restarts them.
//
// @param           t   "test handle used to drive the fake manager"
func Test_RestartAll(t *testing.T) {
	// tracks holds each started handle outside the manager so we can inspect it after RestartAll
	// clears the manager's map. The done channel is ctx.Done(), which cancel() closes.
	tracks := make(map[string]*watcherHandle)
	mgr := &watcherManager{
		watchers: make(map[string]*watcherHandle),
		start: func(repoPath string, envs []string) *watcherHandle {
			ctx, cancel := context.WithCancel(context.Background())
			h := &watcherHandle{done: ctx.Done(), cancel: cancel}
			tracks[repoPath] = h
			return h
		},
	}
	// Populate the manager's map so RestartAll has handles to cancel. Calling mgr.start alone only
	// invokes the function field; reconcile is what stores the result, so we mirror that here.
	mgr.watchers["/repo/a"] = mgr.start("/repo/a", nil)
	mgr.watchers["/repo/b"] = mgr.start("/repo/b", nil)

	mgr.RestartAll()

	assert.Assert(t, len(mgr.watchers) == 0)
	for _, h := range tracks {
		select {
		case <-h.done:
			// done channel closed: cancel was called on this handle.
		default:
			t.Fatalf("watcher handle was not canceled")
		}
	}
}

// @description    Returns a pointer to n.
//
// @param           n      "value to point at"
//
// @return          *int   "pointer to n"
func ptrInt(n int) *int { return &n }

// @description    Returns a pointer to s.
//
// @param           s         "value to point at"
//
// @return          *string   "pointer to s"
func ptrStr(s string) *string { return &s }

// @description    Verifies watchForLocalChange restarts only on [auto-sync] change.
//
// Test_WatchForLocalChange starts watchForLocalChange against a temp repo, writes an unrelated
// .git/config change (a [remote] section) that must not trigger a cancel, then writes an
// [auto-sync] syncInterval change that must trigger a cancel within the poll interval.
//
// @param           t   "test handle used to create the repo and drive the watcher"
func Test_WatchForLocalChange(t *testing.T) {
	// Shorten the poll interval for the test; restored automatically since the test process exits.
	localChangePollInterval = 50 * time.Millisecond
	t.Cleanup(func() { localChangePollInterval = configPollInterval })

	repoPath := t.TempDir()
	repo, err := git.PlainInit(repoPath, false)
	assert.NilError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan struct{}, 1)
	go watchForLocalChange(ctx, func() { cancel(); canceled <- struct{}{} }, logging.WithRepo(repoPath), repoPath)

	// Write an unrelated .git/config change (a new [remote] section). Must not cancel.
	cfg, err := repo.Config()
	assert.NilError(t, err)
	cfg.Raw.Section("remote").Subsection("origin").SetOption("url", "https://example.com/repo")
	assert.NilError(t, repo.SetConfig(cfg))

	// Wait past a poll interval; expect no cancel from the unrelated change.
	select {
	case <-canceled:
		t.Fatal("watcher restarted on unrelated .git/config change")
	case <-time.After(150 * time.Millisecond):
	}

	// Write an [auto-sync] change. Expect cancel.
	assert.NilError(t, config.SetLocalSetting(repoPath, "syncInterval", "33"))
	select {
	case <-canceled:
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not restart on [auto-sync] change")
	}
}
