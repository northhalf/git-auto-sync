package main

import (
	"testing"

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

func newFakeStart() *fakeStart {
	return &fakeStart{handles: make(map[string]chan struct{})}
}

func (f *fakeStart) start(repo string, envs []string) *watcherHandle {
	f.started = append(f.started, fakeStartCall{repo: repo, envs: append([]string(nil), envs...)})
	done := make(chan struct{})
	f.handles[repo] = done
	return &watcherHandle{done: done}
}

// close simulates a watcher exiting for the given repository.
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
