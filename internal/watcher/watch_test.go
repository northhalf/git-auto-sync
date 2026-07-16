package watcher

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/rjeczalik/notify"
	"gotest.tools/v3/assert"
)

type fakeEventInfo struct {
	path string
}

// @description    Returns the fake filesystem event type.
//
// @return          notify.Event  "write event used by watcher tests"
func (f fakeEventInfo) Event() notify.Event {
	return notify.Write
}

// @description    Returns the fake filesystem event path.
//
// @return          string  "configured test path"
func (f fakeEventInfo) Path() string {
	return f.path
}

// @description    Returns no platform-specific event data.
//
// @return          interface{}  "always nil"
func (f fakeEventInfo) Sys() interface{} {
	return nil
}

// @description    Returns a logger that discards test output.
//
// @return          *slog.Logger  "logger backed by io.Discard"
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// @description    Waits for one synchronization attempt.
//
// @param           t      "test handle used to fail on timeout"
//
// @param           calls  "channel that receives synchronization attempt numbers"
//
// @return          int    "received synchronization attempt number"
func waitForSyncCall(t *testing.T, calls <-chan int) int {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for synchronization attempt")
		return 0
	}
}

// @description    Waits for a watcher loop to return.
//
// @param           t     "test handle used to fail on timeout"
//
// @param           done  "channel that receives the watcher result"
func waitForWatchDone(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		assert.NilError(t, err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watcher to stop")
	}
}

// @description    Verifies retry delay selection and capping.
//
// Test_RetryDelay verifies that consecutive failures advance through the configured schedule and
// that failures beyond the schedule continue using its final delay.
//
// @param           t   "test handle used for delay assertions"
func Test_RetryDelay(t *testing.T) {
	delays := []time.Duration{
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		60 * time.Minute,
	}
	want := []time.Duration{
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		15 * time.Minute,
		30 * time.Minute,
		60 * time.Minute,
		60 * time.Minute,
		60 * time.Minute,
	}

	for failure := range want {
		assert.Equal(t, retryDelay(failure, delays), want[failure])
	}
}

// @description    Verifies retrying remote synchronization errors.
//
// Test_WatchLoopRetriesRemoteErrors verifies that the initial synchronization retries consecutive
// remote errors and returns to normal operation after a successful synchronization.
//
// @param           t   "test handle used for watcher lifecycle and attempt assertions"
func Test_WatchLoopRetriesRemoteErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	remoteErr := errors.New("remote failed")
	results := []error{remoteErr, remoteErr, nil}
	calls := make(chan int, len(results))
	var attempts atomic.Int32
	deps := watchDependencies{
		autoSync: func() error {
			attempt := int(attempts.Add(1))
			calls <- attempt
			return results[attempt-1]
		},
		isRemoteSyncError: func(err error) bool { return errors.Is(err, remoteErr) },
		syncErrorStage:    func(error) string { return "fetch" },
		retryDelays:       []time.Duration{time.Millisecond, 2 * time.Millisecond},
	}

	done := make(chan error, 1)
	go func() {
		done <- runWatchLoop(ctx, discardLogger(), time.Millisecond, nil, nil, nil, deps)
	}()

	assert.Equal(t, waitForSyncCall(t, calls), 1)
	assert.Equal(t, waitForSyncCall(t, calls), 2)
	assert.Equal(t, waitForSyncCall(t, calls), 3)

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies retry reset after synchronization recovery.
//
// Test_WatchLoopResetsRetryAfterSuccess verifies that a remote failure after a successful retry uses
// the first retry delay instead of continuing at the previous failure level.
//
// @param           t   "test handle used for watcher lifecycle and retry assertions"
func Test_WatchLoopResetsRetryAfterSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	remoteErr := errors.New("remote failed")
	results := []error{remoteErr, nil, remoteErr, nil}
	calls := make(chan int, len(results))
	ticks := make(chan time.Time)
	var attempts atomic.Int32
	deps := watchDependencies{
		autoSync: func() error {
			attempt := int(attempts.Add(1))
			calls <- attempt
			return results[attempt-1]
		},
		isRemoteSyncError: func(err error) bool { return errors.Is(err, remoteErr) },
		syncErrorStage:    func(error) string { return "fetch" },
		retryDelays:       []time.Duration{time.Millisecond, 100 * time.Millisecond},
	}

	done := make(chan error, 1)
	go func() {
		done <- runWatchLoop(ctx, discardLogger(), time.Millisecond, nil, nil, ticks, deps)
	}()

	assert.Equal(t, waitForSyncCall(t, calls), 1)
	assert.Equal(t, waitForSyncCall(t, calls), 2)
	ticks <- time.Now()
	assert.Equal(t, waitForSyncCall(t, calls), 3)

	select {
	case call := <-calls:
		assert.Equal(t, call, 4)
	case <-time.After(30 * time.Millisecond):
		t.Fatal("retry delay did not reset after successful synchronization")
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies pausing after a non-remote synchronization error.
//
// Test_WatchLoopPausesAfterNonRemoteError verifies that periodic triggers do not start another
// synchronization after the initial synchronization returns an error that requires user action.
//
// @param           t   "test handle used for watcher lifecycle and attempt assertions"
func Test_WatchLoopPausesAfterNonRemoteError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := make(chan int, 2)
	ticks := make(chan time.Time, 3)
	deps := watchDependencies{
		autoSync: func() error {
			calls <- 1
			return errors.New("author failed")
		},
		isRemoteSyncError: func(error) bool { return false },
		syncErrorStage:    func(error) string { return "author" },
		retryDelays:       []time.Duration{time.Millisecond},
	}

	done := make(chan error, 1)
	go func() {
		done <- runWatchLoop(ctx, discardLogger(), time.Millisecond, nil, nil, ticks, deps)
	}()

	assert.Equal(t, waitForSyncCall(t, calls), 1)
	for range 3 {
		ticks <- time.Now()
	}
	select {
	case <-calls:
		t.Fatal("paused watcher started another synchronization")
	case <-time.After(20 * time.Millisecond):
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies trigger coalescing during synchronization.
//
// Test_WatchLoopCoalescesTriggersDuringSync verifies that multiple periodic triggers received while
// one synchronization runs produce one follow-up synchronization.
//
// @param           t   "test handle used for watcher lifecycle and attempt assertions"
func Test_WatchLoopCoalescesTriggersDuringSync(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := make(chan int, 4)
	ticks := make(chan time.Time)
	releaseFirst := make(chan struct{})
	var attempts atomic.Int32
	deps := watchDependencies{
		autoSync: func() error {
			attempt := int(attempts.Add(1))
			calls <- attempt
			if attempt == 1 {
				<-releaseFirst
			}
			return nil
		},
		isRemoteSyncError: func(error) bool { return false },
		syncErrorStage:    func(error) string { return "" },
		retryDelays:       []time.Duration{time.Millisecond},
	}

	done := make(chan error, 1)
	go func() {
		done <- runWatchLoop(ctx, discardLogger(), time.Millisecond, nil, nil, ticks, deps)
	}()

	assert.Equal(t, waitForSyncCall(t, calls), 1)
	for range 3 {
		ticks <- time.Now()
	}
	close(releaseFirst)
	assert.Equal(t, waitForSyncCall(t, calls), 2)

	select {
	case call := <-calls:
		t.Fatalf("coalesced triggers started unexpected synchronization %d", call)
	case <-time.After(20 * time.Millisecond):
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies immediate triggers consume pending file debounce.
//
// Test_WatchLoopImmediateTriggersCancelDebounce verifies that periodic and awake triggers synchronize
// without waiting for the filesystem debounce and prevent its timer from starting a later empty sync.
//
// @param           t   "test handle used for trigger timing and synchronization assertions"
func Test_WatchLoopImmediateTriggersCancelDebounce(t *testing.T) {
	tests := []struct {
		name string
		wake bool
	}{
		{name: "periodic"},
		{name: "awake", wake: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			calls := make(chan int, 3)
			events := make(chan notify.EventInfo)
			awake := make(chan bool)
			ticks := make(chan time.Time)
			var attempts atomic.Int32
			deps := watchDependencies{
				autoSync: func() error {
					calls <- int(attempts.Add(1))
					return nil
				},
				shouldIgnore:      func(string) (bool, error) { return false, nil },
				isRemoteSyncError: func(error) bool { return false },
				syncErrorStage:    func(error) string { return "" },
				retryDelays:       []time.Duration{time.Millisecond},
			}

			done := make(chan error, 1)
			go func() {
				done <- runWatchLoop(ctx, discardLogger(), 100*time.Millisecond, events, awake, ticks, deps)
			}()

			assert.Equal(t, waitForSyncCall(t, calls), 1)
			time.Sleep(5 * time.Millisecond)
			events <- fakeEventInfo{path: "/repo/file"}
			if tt.wake {
				awake <- true
			} else {
				ticks <- time.Now()
			}

			select {
			case call := <-calls:
				assert.Equal(t, call, 2)
			case <-time.After(30 * time.Millisecond):
				t.Fatal("immediate trigger waited for filesystem debounce")
			}

			select {
			case call := <-calls:
				t.Fatalf("canceled debounce started unexpected synchronization %d", call)
			case <-time.After(120 * time.Millisecond):
			}

			cancel()
			waitForWatchDone(t, done)
		})
	}
}

// @description    Verifies cancellation while synchronization is running.
//
// Test_WatchLoopWaitsForRunningSyncOnCancel verifies that cancellation prevents new work but waits
// for the active synchronization to finish before the watcher returns.
//
// @param           t   "test handle used for watcher lifecycle assertions"
func Test_WatchLoopWaitsForRunningSyncOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	deps := watchDependencies{
		autoSync: func() error {
			close(started)
			<-release
			return nil
		},
		isRemoteSyncError: func(error) bool { return false },
		syncErrorStage:    func(error) string { return "" },
		retryDelays:       []time.Duration{time.Millisecond},
	}

	done := make(chan error, 1)
	go func() {
		done <- runWatchLoop(ctx, discardLogger(), time.Millisecond, nil, nil, nil, deps)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for synchronization to start")
	}
	cancel()

	select {
	case <-done:
		t.Fatal("watcher returned before the running synchronization finished")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	waitForWatchDone(t, done)
}

// @description    Verifies initial remote failure isolation.
//
// Test_WatchForChangesKeepsRunningAfterInitialRemoteFailure verifies that a failed initial fetch
// moves the repository watcher into retry backoff instead of returning or terminating the daemon.
//
// @param           t   "test handle used to prepare the repository and assert watcher lifecycle"
func Test_WatchForChangesKeepsRunningAfterInitialRemoteFailure(t *testing.T) {
	repoPath := t.TempDir()
	commands := [][]string{
		{"init", repoPath},
		{"-C", repoPath, "config", "user.email", "watcher@example.com"},
		{"-C", repoPath, "config", "user.name", "Watcher Test"},
		{"-C", repoPath, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing.git")},
	}
	for _, args := range commands {
		cmd := exec.Command("git", args...)
		assert.NilError(t, cmd.Run())
	}

	cfg := config.RepoConfig{
		RepoPath:     repoPath,
		SyncInterval: time.Hour,
		FSLag:        time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- WatchForChanges(ctx, discardLogger(), cfg)
	}()

	select {
	case err := <-done:
		t.Fatalf("watcher returned after retryable initial failure: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies cleanup after an event inspection error.
//
// Test_WatchLoopWaitsForRunningSyncOnInspectionError verifies that an event inspection error stops
// new watcher work but waits for the active synchronization before returning the inspection error.
//
// @param           t   "test handle used for watcher lifecycle and error assertions"
func Test_WatchLoopWaitsForRunningSyncOnInspectionError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	release := make(chan struct{})
	events := make(chan notify.EventInfo, 1)
	inspectErr := errors.New("inspect failed")
	deps := watchDependencies{
		autoSync: func() error {
			close(started)
			<-release
			return nil
		},
		shouldIgnore:      func(string) (bool, error) { return false, inspectErr },
		isRemoteSyncError: func(error) bool { return false },
		syncErrorStage:    func(error) string { return "" },
		retryDelays:       []time.Duration{time.Millisecond},
	}

	done := make(chan error, 1)
	go func() {
		done <- runWatchLoop(ctx, discardLogger(), time.Millisecond, events, nil, nil, deps)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for synchronization to start")
	}
	events <- fakeEventInfo{path: "/repo/file"}

	select {
	case <-done:
		t.Fatal("watcher returned before the running synchronization finished")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-done:
		assert.Assert(t, errors.Is(err, inspectErr))
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watcher inspection error")
	}
}
