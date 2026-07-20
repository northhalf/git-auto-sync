package watcher

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/rjeczalik/notify"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/syncer"
)

const (
	watchModeNormal     = iota
	watchModeBackingOff // remote sync error, need to wait
	watchModePaused     // unrecoverable error, paused watch loop
)

// StateReport is the runtime status a watcher reports on state transitions. Paused is true when the
// watcher halted after a non-remote error needing user intervention; Stage carries the failed
// synchronization stage in that case and is empty while running. Remote-failure backoff is reported
// as running because it auto-recovers. LastSyncedAt carries the time of the most recent successful
// sync on a running report that follows a successful AutoSync, and the zero time on the watcher's
// initial report and on paused reports, so a never-synced repository reads as "never synced".
type StateReport struct {
	Paused       bool
	Stage        string
	LastSyncedAt time.Time
}

// watcherRetryDelays is the capped retry schedule for consecutive remote synchronization failures.
// Tests replace it with a millisecond schedule.
var watcherRetryDelays = []time.Duration{
	2 * time.Minute,
	4 * time.Minute,
	8 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	60 * time.Minute,
}

// @description    Synchronizes and watches a repository.
//
// WatchForChanges starts filesystem and wake notifications before the initial synchronization, then
// runs a repository-local state machine. Fetch and push errors use capped retry backoff; errors from
// other synchronization stages pause the repository until the watcher context is canceled. A
// failure in one repository never terminates the daemon process or another repository watcher.
// reportState receives state transitions for the caller to record; pass nil when no recording is needed,
// such as the foreground CLI watcher.
//
// @param           ctx      "context whose cancellation stops the watcher and releases its resources"
//
// @param           logger   "repository-scoped logger"
//
// @param           cfg      "repository configuration and watcher timing values"
//
// @param           reportState  "callback invoked on watcher state transitions, or nil"
//
// @return          error    "an error from watcher setup or filesystem event inspection"
func WatchForChanges(ctx context.Context, logger *slog.Logger, cfg config.RepoConfig, reportState func(StateReport)) error {
	logger.Debug("starting watcher")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	notifyChannel := make(chan notify.EventInfo, 100)
	if err := notify.Watch(filepath.Join(cfg.RepoPath, "..."), notifyChannel, notify.Write, notify.Rename, notify.Remove, notify.Create); err != nil {
		logger.Error("watcher failed", "operation", "watch filesystem", "error", err)
		return err
	}
	defer notify.Stop(notifyChannel)

	awakeChannel := make(chan bool, 100)
	notifier, err := NewAwakeNotifier(logger)
	if err != nil {
		logger.Error("watcher failed", "operation", "create awake notifier", "error", err)
		return err
	}
	if err := notifier.Start(ctx, awakeChannel); err != nil {
		logger.Error("watcher failed", "operation", "start awake notifier", "error", err)
		return err
	}

	syncTicker := time.NewTicker(cfg.SyncInterval)
	defer syncTicker.Stop()

	logger.Info("watcher started")
	if err := runWatchLoop(ctx, logger, cfg, reportState, notifyChannel, awakeChannel, syncTicker.C); err != nil {
		return err
	}
	return nil
}

// @description    Selects a retry delay for a consecutive remote failure.
//
// @param           failure  "zero-based consecutive remote failure index"
//
// @param           delays   "ordered retry delay schedule"
//
// @return          time.Duration  "selected delay, capped at the final schedule entry"
func retryDelay(failure int, delays []time.Duration) time.Duration {
	if len(delays) == 0 {
		return 0
	}
	if failure >= len(delays) {
		return delays[len(delays)-1]
	}
	return delays[failure]
}

// @description    Runs the repository synchronization state machine.
//
// runWatchLoop performs the initial synchronization, consumes filesystem, awake, and periodic
// triggers, retries remote synchronization errors with the configured delay schedule, and pauses
// after errors that require user intervention. It runs at most one synchronization at a time and
// waits for an active synchronization when ctx is canceled.
//
// @param           ctx           "context whose cancellation stops the watcher loop"
//
// @param           logger        "repository-scoped logger"
//
// @param           cfg           "repository configuration; Debounce delays filesystem and wake triggers"
//
// @param           reportState   "receives state transitions, or nil when the caller does not record state"
//
// @param           notifyEvents  "filesystem events, or nil when no filesystem source is attached"
//
// @param           awakeEvents   "platform wake events, or nil when wake notifications are unavailable"
//
// @param           syncTicks     "periodic synchronization ticks, or nil when periodic sync is disabled"
//
// @return          returnErr  "nil on cancellation, or an event inspection error retained until the active sync finishes"
func runWatchLoop(
	ctx context.Context,
	logger *slog.Logger,
	cfg config.RepoConfig,
	reportState func(StateReport),
	notifyEvents <-chan notify.EventInfo,
	awakeEvents <-chan bool,
	syncTicks <-chan time.Time,
) (returnErr error) {
	// mode records whether the watcher accepts normal triggers, waits for a remote retry, or remains paused.
	mode := watchModeNormal
	// remoteFailures selects the next delay from the capped retry schedule and resets after a successful sync.
	remoteFailures := 0
	// syncRunning prevents overlapping AutoSync calls.
	syncRunning := false
	// pendingSync coalesces triggers received while a sync is running or the watcher is backing off.
	pendingSync := false
	// canceling stops new work while the loop waits for an active sync to finish.
	canceling := false

	// debounceTimer delays filesystem and wake triggers until writes have settled.
	var debounceTimer *time.Timer
	// debounceC is nil when the debounce timer must not participate in the select loop.
	var debounceC <-chan time.Time
	// retryTimer schedules the next sync after a remote-stage failure.
	var retryTimer *time.Timer
	// retryC is nil while no remote retry is scheduled.
	var retryC <-chan time.Time

	stopTimer := func(timer *time.Timer) {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}

	syncResults := make(chan error, 1)
	startSync := func() {
		if syncRunning || canceling || mode == watchModePaused {
			return
		}
		syncRunning = true
		pendingSync = false
		go func() {
			syncResults <- syncer.AutoSync(logger, cfg)
		}()
	}

	requestDebouncedSync := func() {
		if syncRunning {
			pendingSync = true
			return
		}
		if cfg.Debounce <= 0 {
			startSync()
			return
		}
		stopTimer(debounceTimer)
		if debounceTimer == nil {
			debounceTimer = time.NewTimer(cfg.Debounce)
		} else {
			debounceTimer.Reset(cfg.Debounce)
		}
		debounceC = debounceTimer.C
	}

	requestImmediateSync := func() {
		stopTimer(debounceTimer)
		debounceC = nil
		if syncRunning {
			pendingSync = true
			return
		}
		startSync()
	}

	// report forwards a state transition to the optional recorder. It is nil-safe so the foreground
	// CLI watcher, which passes no recorder, runs unchanged.
	report := func(state StateReport) {
		if reportState != nil {
			reportState(state)
		}
	}

	startSync()
	report(StateReport{Paused: false})
	ctxDone := ctx.Done()

	for {
		select {
		case <-ctxDone:
			canceling = true
			ctxDone = nil
			notifyEvents = nil
			awakeEvents = nil
			syncTicks = nil
			stopTimer(debounceTimer)
			debounceC = nil
			stopTimer(retryTimer)
			retryC = nil
			if !syncRunning {
				return nil
			}

		case err := <-syncResults:
			syncRunning = false
			if canceling {
				return returnErr
			}
			if err == nil {
				if remoteFailures > 0 {
					logger.Info("sync recovered", "consecutive_failures", remoteFailures)
				}
				mode = watchModeNormal
				remoteFailures = 0
				report(StateReport{Paused: false, LastSyncedAt: time.Now()})
				if pendingSync {
					pendingSync = false
					requestDebouncedSync()
				}
				continue
			}

			stage := syncer.SyncErrorStage(err)
			if syncer.IsRemoteSyncError(err) {
				mode = watchModeBackingOff
				delay := retryDelay(remoteFailures, watcherRetryDelays)
				remoteFailures++
				logger.Warn("sync backing off", "stage", stage, "retry_in", delay, "consecutive_failures", remoteFailures)
				stopTimer(retryTimer)
				if retryTimer == nil {
					retryTimer = time.NewTimer(delay)
				} else {
					retryTimer.Reset(delay)
				}
				retryC = retryTimer.C
				continue
			}

			mode = watchModePaused
			pendingSync = false
			report(StateReport{Paused: true, Stage: stage})
			stopTimer(debounceTimer)
			debounceC = nil
			stopTimer(retryTimer)
			retryC = nil
			logger.Error("sync paused", "stage", stage, "recovery", "restart daemon or remove and re-add repository")

		case <-retryC:
			retryC = nil
			if mode == watchModeBackingOff {
				startSync()
			}

		case <-debounceC:
			debounceC = nil
			if mode == watchModeNormal {
				startSync()
			}

		case ei, ok := <-notifyEvents:
			if !ok {
				notifyEvents = nil
				continue
			}
			if mode == watchModePaused {
				continue
			}
			if mode == watchModeBackingOff {
				pendingSync = true
				continue
			}
			ignore, err := syncer.ShouldIgnoreFile(cfg.RepoPath, ei.Path())
			if err != nil {
				logger.Error("watcher failed", "operation", "inspect filesystem event", "path", ei.Path(), "error", err)
				returnErr = err
				canceling = true
				ctxDone = nil
				notifyEvents = nil
				awakeEvents = nil
				syncTicks = nil
				stopTimer(debounceTimer)
				debounceC = nil
				stopTimer(retryTimer)
				retryC = nil
				if !syncRunning {
					return returnErr
				}
				continue
			}
			if ignore {
				logger.Debug("filesystem event skipped", "path", ei.Path())
				continue
			}
			requestDebouncedSync()

		case _, ok := <-awakeEvents:
			if !ok {
				awakeEvents = nil
				continue
			}
			switch mode {
			case watchModeNormal:
				requestImmediateSync()
			case watchModeBackingOff:
				pendingSync = true
			case watchModePaused:
				continue
			}

		case _, ok := <-syncTicks:
			if !ok {
				syncTicks = nil
				continue
			}
			switch mode {
			case watchModeNormal:
				requestImmediateSync()
			case watchModeBackingOff:
				pendingSync = true
			case watchModePaused:
				continue
			}
		}
	}
}
