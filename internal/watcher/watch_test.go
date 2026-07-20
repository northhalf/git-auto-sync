package watcher

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/rjeczalik/notify"
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

// logRecord is one captured log entry: its message and top-level attributes.
type logRecord struct {
	msg   string
	attrs map[string]any
}

// captureHandler collects every log record so tests can assert watcher behavior through the
// records the loop emits, such as backoff warnings and sync completions.
type captureHandler struct {
	mu      sync.Mutex
	records []logRecord
}

// @description    Reports all levels as enabled so debug records are captured.
//
// @return          bool  "always true"
func (h *captureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

// @description    Stores one record with its attributes.
//
// @param           _  "record context, unused"
//
// @param           r  "log record to capture"
//
// @return          error  "always nil"
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := logRecord{msg: r.Message, attrs: make(map[string]any)}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, rec)
	h.mu.Unlock()
	return nil
}

// @description    Returns the handler unchanged; captured records need no pre-bound attributes.
//
// @return          slog.Handler  "the same handler"
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

// @description    Returns the handler unchanged; grouping is irrelevant to capture.
//
// @return          slog.Handler  "the same handler"
func (h *captureHandler) WithGroup(string) slog.Handler {
	return h
}

// @description    Counts captured records with the given message.
//
// @param           msg  "exact log message to count"
//
// @return          int  "number of matching records"
func (h *captureHandler) count(msg string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.msg == msg {
			n++
		}
	}
	return n
}

// @description    Returns the nth captured record with the given message.
//
// @param           msg  "exact log message to find"
//
// @param           n    "zero-based index among matching records"
//
// @return          logRecord  "the matching record"
//
// @return          bool       "true when the record exists"
func (h *captureHandler) record(msg string, n int) (logRecord, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.msg == msg {
			if n == 0 {
				return r, true
			}
			n--
		}
	}
	return logRecord{}, false
}

// @description    Waits until cond reports true or the timeout elapses.
//
// @param           t        "test handle used to fail on timeout"
//
// @param           timeout  "maximum wait duration"
//
// @param           what     "description of the awaited condition for timeout messages"
//
// @param           cond     "condition polled every few milliseconds"
func waitForCondition(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// @description    Waits until the handler captures at least n records with the message.
//
// @param           t     "test handle used to fail on timeout"
//
// @param           h     "capturing log handler"
//
// @param           msg   "exact log message to await"
//
// @param           n     "required record count"
func waitForLog(t *testing.T, h *captureHandler, msg string, n int) {
	t.Helper()
	waitForCondition(t, 15*time.Second, msg, func() bool { return h.count(msg) >= n })
}

// @description    Waits for one watcher state report.
//
// @param           t        "test handle used to fail on timeout"
//
// @param           reports  "channel that receives state reports"
//
// @return          StateReport  "received state report"
func waitForReport(t *testing.T, reports <-chan StateReport) StateReport {
	t.Helper()
	select {
	case r := <-reports:
		return r
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for state report")
		return StateReport{}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for watcher to stop")
	}
}

// @description    Runs a git command and fails the test when it fails.
//
// @param           t     "test handle used to report command failure"
//
// @param           args  "git command arguments"
func gitCLI(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// @description    Builds a repository with one commit and, optionally, a seeded bare upstream.
//
// newLoopRepo creates repoPath with an initial commit on the main branch. When upstream is true,
// it also creates a bare remote, pushes main, and sets the upstream so AutoSync can fetch, and
// returns the remote path for tests that toggle remote availability with os.Rename.
//
// @param           t         "test handle used for repository setup"
//
// @param           upstream  "whether to create and configure a reachable bare upstream"
//
// @return          string    "repository path"
//
// @return          string    "bare remote path, or empty when upstream is false"
func newLoopRepo(t *testing.T, upstream bool) (string, string) {
	t.Helper()
	repoPath := t.TempDir()
	gitCLI(t, "init", "-b", "main", repoPath)
	gitCLI(t, "-C", repoPath, "config", "user.email", "watcher@example.com")
	gitCLI(t, "-C", repoPath, "config", "user.name", "Watcher Test")
	if err := os.WriteFile(filepath.Join(repoPath, "file.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error %v", err)
	}
	gitCLI(t, "-C", repoPath, "add", "file.txt")
	gitCLI(t, "-C", repoPath, "commit", "-m", "initial")

	if !upstream {
		return repoPath, ""
	}
	remotePath := filepath.Join(t.TempDir(), "remote.git")
	gitCLI(t, "init", "--bare", remotePath)
	gitCLI(t, "-C", repoPath, "remote", "add", "origin", remotePath)
	gitCLI(t, "-C", repoPath, "push", "-u", "origin", "main")
	return repoPath, remotePath
}

// @description    Replaces the watcher retry schedule for the duration of a test.
//
// @param           t       "test handle used to register cleanup"
//
// @param           delays  "millisecond-scale retry schedule"
func swapRetryDelays(t *testing.T, delays []time.Duration) {
	t.Helper()
	original := watcherRetryDelays
	watcherRetryDelays = delays
	t.Cleanup(func() { watcherRetryDelays = original })
}

// @description    Writes a git wrapper script that slows every invocation to stretch syncs.
//
// slowGitPath returns an executable suitable for RepoConfig.GitExec: each git subprocess sleeps
// before running, so a test can observe the watcher while a synchronization is in flight.
//
// @param           t  "test handle used to create the wrapper and skip on Windows"
//
// @return          string  "path of the slow-git wrapper"
func slowGitPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("slow-git wrapper requires a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "slow-git")
	script := "#!/bin/sh\nsleep 0.5\nexec git \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) returned error %v", path, err)
	}
	return path
}

// @description    Starts a watcher loop in the background over the test repository.
//
// @param           t           "test handle used for lifecycle assertions"
//
// @param           ctx         "context controlling the loop"
//
// @param           logger      "capturing logger"
//
// @param           cfg         "repository configuration for the loop"
//
// @param           reportState "state report callback, or nil"
//
// @param           events      "filesystem events channel, or nil"
//
// @param           awake       "wake events channel, or nil"
//
// @param           ticks       "periodic ticks channel, or nil"
//
// @return          chan error  "receives the loop result once it returns"
func startLoop(t *testing.T, ctx context.Context, logger *slog.Logger, cfg config.RepoConfig, reportState func(StateReport), events chan notify.EventInfo, awake chan bool, ticks chan time.Time) chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		done <- runWatchLoop(ctx, logger, cfg, reportState, events, awake, ticks)
	}()
	return done
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
		if retryDelay(failure, delays) != want[failure] {
			t.Fatalf("got %v, want %v", retryDelay(failure, delays), want[failure])
		}
	}
}

// @description    Verifies retrying remote synchronization errors.
//
// Test_WatchLoopRetriesRemoteErrors verifies that an unreachable upstream retries with the
// configured backoff schedule and returns to normal operation once the remote is reachable again.
//
// @param           t   "test handle used for repository setup and backoff assertions"
func Test_WatchLoopRetriesRemoteErrors(t *testing.T) {
	repoPath, remotePath := newLoopRepo(t, true)
	deadPath := remotePath + ".dead"
	if err := os.Rename(remotePath, deadPath); err != nil {
		t.Fatalf("Rename returned error %v", err)
	}
	swapRetryDelays(t, []time.Duration{50 * time.Millisecond, 50 * time.Millisecond})

	handler := &captureHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.RepoConfig{RepoPath: repoPath, Debounce: time.Millisecond}
	done := startLoop(t, ctx, slog.New(handler), cfg, nil, nil, nil, nil)

	waitForLog(t, handler, "sync backing off", 2)
	for i, failures := range []int64{1, 2} {
		rec, ok := handler.record("sync backing off", i)
		if !ok {
			t.Fatalf("missing backoff record %d", i)
		}
		if rec.attrs["stage"] != "fetch" {
			t.Fatalf("backoff %d stage = %v, want fetch", i, rec.attrs["stage"])
		}
		if rec.attrs["consecutive_failures"] != failures {
			t.Fatalf("backoff %d consecutive_failures = %v, want %d", i, rec.attrs["consecutive_failures"], failures)
		}
	}

	if err := os.Rename(deadPath, remotePath); err != nil {
		t.Fatalf("Rename returned error %v", err)
	}
	waitForLog(t, handler, "sync recovered", 1)
	rec, _ := handler.record("sync recovered", 0)
	if rec.attrs["consecutive_failures"] != int64(2) {
		t.Fatalf("recovered consecutive_failures = %v, want 2", rec.attrs["consecutive_failures"])
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies retry reset after synchronization recovery.
//
// Test_WatchLoopResetsRetryAfterSuccess verifies that a remote failure after a successful retry
// uses the first retry delay instead of continuing at the previous failure level.
//
// @param           t   "test handle used for repository setup and retry-delay assertions"
func Test_WatchLoopResetsRetryAfterSuccess(t *testing.T) {
	repoPath, remotePath := newLoopRepo(t, true)
	deadPath := remotePath + ".dead"
	if err := os.Rename(remotePath, deadPath); err != nil {
		t.Fatalf("Rename returned error %v", err)
	}
	firstDelay := 400 * time.Millisecond
	swapRetryDelays(t, []time.Duration{firstDelay, 10 * time.Second})

	handler := &captureHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.RepoConfig{RepoPath: repoPath, Debounce: time.Millisecond}
	ticks := make(chan time.Time)
	done := startLoop(t, ctx, slog.New(handler), cfg, nil, nil, nil, ticks)

	waitForLog(t, handler, "sync backing off", 1)
	if err := os.Rename(deadPath, remotePath); err != nil {
		t.Fatalf("Rename returned error %v", err)
	}
	waitForLog(t, handler, "sync recovered", 1)

	if err := os.Rename(remotePath, deadPath); err != nil {
		t.Fatalf("Rename returned error %v", err)
	}
	// Recovery left the watcher idle, so trigger the synchronization that fails again.
	ticks <- time.Now()
	waitForLog(t, handler, "sync backing off", 2)
	rec, ok := handler.record("sync backing off", 1)
	if !ok {
		t.Fatal("missing second backoff record")
	}
	if rec.attrs["retry_in"] != firstDelay {
		t.Fatalf("retry_in after recovery = %v, want the first delay %v", rec.attrs["retry_in"], firstDelay)
	}

	if err := os.Rename(deadPath, remotePath); err != nil {
		t.Fatalf("Rename returned error %v", err)
	}
	waitForLog(t, handler, "sync recovered", 2)

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies pausing after a non-remote synchronization error.
//
// Test_WatchLoopPausesAfterNonRemoteError verifies that a repository without an upstream pauses
// and that periodic triggers do not start another synchronization afterwards.
//
// @param           t   "test handle used for repository setup and pause assertions"
func Test_WatchLoopPausesAfterNonRemoteError(t *testing.T) {
	repoPath, _ := newLoopRepo(t, false)
	reports := make(chan StateReport, 4)
	ticks := make(chan time.Time)

	handler := &captureHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.RepoConfig{RepoPath: repoPath, Debounce: time.Millisecond}
	done := startLoop(t, ctx, slog.New(handler), cfg, func(r StateReport) { reports <- r }, nil, nil, ticks)

	if r := waitForReport(t, reports); r.Paused {
		t.Fatalf("initial report Paused = true, want running")
	}
	r := waitForReport(t, reports)
	if !r.Paused {
		t.Fatal("report after no-upstream error Paused = false, want paused")
	}
	if r.Stage != "no-upstream" {
		t.Fatalf("paused stage = %q, want %q", r.Stage, "no-upstream")
	}

	for range 3 {
		ticks <- time.Now()
	}
	time.Sleep(150 * time.Millisecond)
	if got := handler.count("sync paused"); got != 1 {
		t.Fatalf("sync paused records = %d, want 1", got)
	}
	select {
	case r := <-reports:
		t.Fatalf("paused watcher reported an unexpected state: %+v", r)
	default:
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies trigger coalescing during synchronization.
//
// Test_WatchLoopCoalescesTriggersDuringSync verifies that multiple periodic triggers received
// while one synchronization runs produce one follow-up synchronization.
//
// @param           t   "test handle used for repository setup and completion assertions"
func Test_WatchLoopCoalescesTriggersDuringSync(t *testing.T) {
	repoPath, _ := newLoopRepo(t, true)
	ticks := make(chan time.Time)

	handler := &captureHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.RepoConfig{RepoPath: repoPath, GitExec: slowGitPath(t), Debounce: time.Millisecond}
	done := startLoop(t, ctx, slog.New(handler), cfg, nil, nil, nil, ticks)

	waitForLog(t, handler, "starting git command", 1)
	for range 3 {
		ticks <- time.Now()
	}
	waitForLog(t, handler, "sync completed", 2)

	time.Sleep(300 * time.Millisecond)
	if got := handler.count("sync completed"); got != 2 {
		t.Fatalf("sync completed records = %d, want 2 (coalesced triggers)", got)
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies immediate triggers consume pending file debounce.
//
// Test_WatchLoopImmediateTriggersCancelDebounce verifies that periodic and awake triggers
// synchronize without waiting for the filesystem debounce and prevent its timer from starting a
// later empty sync.
//
// @param           t   "test handle used for repository setup and timing assertions"
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
			repoPath, _ := newLoopRepo(t, true)
			events := make(chan notify.EventInfo)
			awake := make(chan bool)
			ticks := make(chan time.Time)

			handler := &captureHandler{}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cfg := config.RepoConfig{RepoPath: repoPath, Debounce: 600 * time.Millisecond}
			done := startLoop(t, ctx, slog.New(handler), cfg, nil, events, awake, ticks)

			waitForLog(t, handler, "sync completed", 1)

			triggerFile := filepath.Join(repoPath, "trigger.txt")
			if err := os.WriteFile(triggerFile, []byte("trigger\n"), 0o644); err != nil {
				t.Fatalf("WriteFile returned error %v", err)
			}
			events <- fakeEventInfo{path: triggerFile}
			if tt.wake {
				awake <- true
			} else {
				ticks <- time.Now()
			}

			// The immediate trigger must not wait for the 600ms debounce.
			waitForCondition(t, 550*time.Millisecond, "immediate sync", func() bool {
				return handler.count("sync completed") >= 2
			})

			// The canceled debounce timer must not start another sync after it would have fired.
			time.Sleep(900 * time.Millisecond)
			if got := handler.count("sync completed"); got != 2 {
				t.Fatalf("sync completed records = %d, want 2 (debounce canceled)", got)
			}

			cancel()
			waitForWatchDone(t, done)
		})
	}
}

// @description    Verifies cancellation while synchronization is running.
//
// Test_WatchLoopWaitsForRunningSyncOnCancel verifies that cancellation prevents new work but
// waits for the active synchronization to finish before the watcher returns.
//
// @param           t   "test handle used for repository setup and lifecycle assertions"
func Test_WatchLoopWaitsForRunningSyncOnCancel(t *testing.T) {
	repoPath, _ := newLoopRepo(t, true)

	handler := &captureHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	cfg := config.RepoConfig{RepoPath: repoPath, GitExec: slowGitPath(t), Debounce: time.Millisecond}
	done := startLoop(t, ctx, slog.New(handler), cfg, nil, nil, nil, nil)

	waitForLog(t, handler, "starting git command", 1)
	cancel()

	select {
	case <-done:
		t.Fatal("watcher returned before the running synchronization finished")
	case <-time.After(200 * time.Millisecond):
	}

	waitForWatchDone(t, done)
}

// @description    Verifies initial remote failure isolation.
//
// Test_WatchForChangesKeepsRunningAfterInitialRemoteFailure verifies that a failed initial fetch
// moves the repository watcher into retry backoff instead of returning or terminating the daemon.
//
// @param           t   "test handle used to prepare the repository and assert watcher lifecycle"
func Test_WatchForChangesKeepsRunningAfterInitialRemoteFailure(t *testing.T) {
	repoPath, _ := newLoopRepo(t, false)
	gitCLI(t, "-C", repoPath, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing.git"))
	gitCLI(t, "-C", repoPath, "config", "branch.main.remote", "origin")
	gitCLI(t, "-C", repoPath, "config", "branch.main.merge", "refs/heads/main")

	handler := &captureHandler{}
	cfg := config.RepoConfig{
		RepoPath:     repoPath,
		SyncInterval: time.Hour,
		Debounce:     time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- WatchForChanges(ctx, slog.New(handler), cfg, nil)
	}()

	waitForLog(t, handler, "sync backing off", 1)
	select {
	case err := <-done:
		t.Fatalf("watcher returned after retryable initial failure: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies state transitions are reported.
//
// Test_WatchLoopReportsStateTransitions verifies that the watcher reports running at start and
// paused (with the failed stage) after a non-remote synchronization error, so the daemon can
// persist the per-repository status for the CLI.
//
// @param           t   "test handle used for repository setup and report assertions"
func Test_WatchLoopReportsStateTransitions(t *testing.T) {
	repoPath, _ := newLoopRepo(t, false)
	reports := make(chan StateReport, 4)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.RepoConfig{RepoPath: repoPath, Debounce: time.Millisecond}
	done := startLoop(t, ctx, discardLogger(), cfg, func(r StateReport) { reports <- r }, nil, nil, nil)

	start := waitForReport(t, reports)
	if start.Paused {
		t.Fatal("initial report Paused = true, want running")
	}
	if start.Stage != "" {
		t.Fatalf("initial report Stage = %q, want empty", start.Stage)
	}
	if !start.LastSyncedAt.IsZero() {
		t.Fatalf("initial report LastSyncedAt = %v, want zero", start.LastSyncedAt)
	}

	paused := waitForReport(t, reports)
	if !paused.Paused {
		t.Fatal("report after no-upstream error Paused = false, want paused")
	}
	if paused.Stage != "no-upstream" {
		t.Fatalf("paused stage = %q, want %q", paused.Stage, "no-upstream")
	}
	if !paused.LastSyncedAt.IsZero() {
		t.Fatalf("paused report LastSyncedAt = %v, want zero", paused.LastSyncedAt)
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies a successful synchronization reports its completion time.
//
// Test_WatchLoopReportsLastSyncedAt verifies that the running report following a successful
// synchronization carries a non-zero LastSyncedAt so the CLI can display the last sync time.
//
// @param           t   "test handle used for repository setup and report assertions"
func Test_WatchLoopReportsLastSyncedAt(t *testing.T) {
	repoPath, _ := newLoopRepo(t, true)
	reports := make(chan StateReport, 4)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.RepoConfig{RepoPath: repoPath, Debounce: time.Millisecond}
	done := startLoop(t, ctx, discardLogger(), cfg, func(r StateReport) { reports <- r }, nil, nil, nil)

	if r := waitForReport(t, reports); r.Paused {
		t.Fatal("initial report Paused = true, want running")
	}
	synced := waitForReport(t, reports)
	if synced.Paused {
		t.Fatal("report after successful sync Paused = true, want running")
	}
	if synced.LastSyncedAt.IsZero() {
		t.Fatal("synced report LastSyncedAt is zero, want the completion time")
	}

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies ignored files do not trigger synchronization.
//
// Test_WatchLoopSkipsIgnoredFiles verifies that an editor swap file event is skipped while a
// regular file event starts a synchronization.
//
// @param           t   "test handle used for repository setup and ignore assertions"
func Test_WatchLoopSkipsIgnoredFiles(t *testing.T) {
	repoPath, _ := newLoopRepo(t, true)
	events := make(chan notify.EventInfo)

	handler := &captureHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.RepoConfig{RepoPath: repoPath, Debounce: time.Millisecond}
	done := startLoop(t, ctx, slog.New(handler), cfg, nil, events, nil, nil)

	waitForLog(t, handler, "sync completed", 1)

	swapFile := filepath.Join(repoPath, ".file.txt.swp")
	if err := os.WriteFile(swapFile, []byte("swap\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error %v", err)
	}
	events <- fakeEventInfo{path: swapFile}
	waitForLog(t, handler, "filesystem event skipped", 1)
	time.Sleep(150 * time.Millisecond)
	if got := handler.count("sync completed"); got != 1 {
		t.Fatalf("sync completed records = %d, want 1 after ignored event", got)
	}

	regularFile := filepath.Join(repoPath, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error %v", err)
	}
	events <- fakeEventInfo{path: regularFile}
	waitForLog(t, handler, "sync completed", 2)

	cancel()
	waitForWatchDone(t, done)
}

// @description    Verifies cleanup after an event inspection error.
//
// Test_WatchLoopWaitsForRunningSyncOnInspectionError verifies that an event inspection error
// stops new watcher work but waits for the active synchronization before returning the error.
//
// @param           t   "test handle used for repository setup and error assertions"
func Test_WatchLoopWaitsForRunningSyncOnInspectionError(t *testing.T) {
	repoPath, _ := newLoopRepo(t, true)
	events := make(chan notify.EventInfo)

	handler := &captureHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.RepoConfig{RepoPath: repoPath, GitExec: slowGitPath(t), Debounce: time.Millisecond}
	done := startLoop(t, ctx, slog.New(handler), cfg, nil, events, nil, nil)

	waitForLog(t, handler, "starting git command", 1)
	if err := os.RemoveAll(filepath.Join(repoPath, ".git")); err != nil {
		t.Fatalf("RemoveAll returned error %v", err)
	}
	events <- fakeEventInfo{path: filepath.Join(repoPath, "file.txt")}

	select {
	case <-done:
		t.Fatal("watcher returned before the running synchronization finished")
	case <-time.After(200 * time.Millisecond):
	}

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("watcher returned nil error, want the inspection error")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for watcher inspection error")
	}
}
