package main

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/logging"
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
// start appends the call to the fake's history and returns a handle whose done channel closes when
// close is called for that repository or the handle's cancel function is invoked, mirroring a real
// watcher exiting on cancellation.
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
	var once sync.Once
	return &watcherHandle{done: done, cancel: func() { once.Do(func() { close(done) }) }}
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

// @description    Reports whether the fake watcher for repo has exited.
//
// @param           repo   "repository path whose done channel is inspected"
//
// @return          bool   "true when the watcher's done channel is closed"
func (f *fakeStart) exited(repo string) bool {
	select {
	case <-f.handles[repo]:
		return true
	default:
		return false
	}
}

// @description    Verifies watcherManager reconciliation.
//
// Test_WatcherManagerReconcile verifies that reconcile starts a watcher for each configured
// repository, does not duplicate running watchers, cancels a watcher whose repository leaves the
// configuration, cleans up exited watchers, and starts a fresh watcher for a repository that is
// re-added after its previous watcher exited.
//
// @param           t   "test handle used for assertions"
func Test_WatcherManagerReconcile(t *testing.T) {
	fs := newFakeStart()
	mgr := &watcherManager{watchers: make(map[string]*watcherHandle), start: fs.start}

	// Initial reconcile starts /a and /b.
	mgr.reconcile([]string{"/a", "/b"}, []string{"K=V"})
	if len(fs.started) != 2 {
		t.Fatalf("got %v, want %v", len(fs.started), 2)
	}
	if fs.started[0].repo != "/a" {
		t.Fatalf("got %v, want %v", fs.started[0].repo, "/a")
	}
	if fs.started[1].repo != "/b" {
		t.Fatalf("got %v, want %v", fs.started[1].repo, "/b")
	}
	if !reflect.DeepEqual(fs.started[0].envs, []string{"K=V"}) {
		t.Fatalf("got %#v, want %#v", fs.started[0].envs, []string{"K=V"})
	}

	// Reconciling the same set starts nothing new and cancels nothing.
	mgr.reconcile([]string{"/a", "/b"}, []string{"K=V"})
	if len(fs.started) != 2 {
		t.Fatalf("got %v, want %v", len(fs.started), 2)
	}
	if fs.exited("/a") {
		t.Fatalf("assertion failed: !fs.exited(\"/a\")")
	}
	if fs.exited("/b") {
		t.Fatalf("assertion failed: !fs.exited(\"/b\")")
	}

	// /b is removed from the config: reconcile cancels its watcher without restarting anything.
	mgr.reconcile([]string{"/a"}, []string{"K=V"})
	if len(fs.started) != 2 {
		t.Fatalf("got %v, want %v", len(fs.started), 2)
	}
	if !fs.exited("/b") {
		t.Fatalf("assertion failed: fs.exited(\"/b\")")
	}
	if fs.exited("/a") {
		t.Fatalf("assertion failed: !fs.exited(\"/a\")")
	}

	// The canceled /b handle is cleaned up on the next pass.
	mgr.reconcile([]string{"/a"}, []string{"K=V"})
	if len(fs.started) != 2 {
		t.Fatalf("got %v, want %v", len(fs.started), 2)
	}

	// Re-adding /b starts a fresh watcher.
	mgr.reconcile([]string{"/a", "/b"}, []string{"K=V"})
	if len(fs.started) != 3 {
		t.Fatalf("got %v, want %v", len(fs.started), 3)
	}
	if fs.started[2].repo != "/b" {
		t.Fatalf("got %v, want %v", fs.started[2].repo, "/b")
	}

	// /a removed from the config: reconcile cancels it; /b keeps running.
	mgr.reconcile([]string{"/b"}, []string{"K=V"})
	if len(fs.started) != 3 {
		t.Fatalf("got %v, want %v", len(fs.started), 3)
	}
	if !fs.exited("/a") {
		t.Fatalf("assertion failed: fs.exited(\"/a\")")
	}
	if fs.exited("/b") {
		t.Fatalf("assertion failed: !fs.exited(\"/b\")")
	}

	// Next pass cleans /a up; nothing is started.
	mgr.reconcile([]string{"/b"}, []string{"K=V"})
	if len(fs.started) != 3 {
		t.Fatalf("got %v, want %v", len(fs.started), 3)
	}
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

	if len(mgr.watchers) != 0 {
		t.Fatalf("assertion failed: len(mgr.watchers) == 0")
	}
	for _, h := range tracks {
		select {
		case <-h.done:
			// done channel closed: cancel was called on this handle.
		default:
			t.Fatalf("watcher handle was not canceled")
		}
	}
}

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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan struct{}, 1)
	go watchForLocalChange(ctx, func() { cancel(); canceled <- struct{}{} }, logging.WithRepo(repoPath), repoPath)

	// Write an unrelated .git/config change (a new [remote] section). Must not cancel.
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg.Raw.Section("remote").Subsection("origin").SetOption("url", "https://example.com/repo")
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait past a poll interval; expect no cancel from the unrelated change.
	select {
	case <-canceled:
		t.Fatal("watcher restarted on unrelated .git/config change")
	case <-time.After(150 * time.Millisecond):
	}

	// Write an [auto-sync] change. Expect cancel.
	if err := config.SetLocalSetting(repoPath, "syncInterval", "33"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case <-canceled:
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not restart on [auto-sync] change")
	}
}
