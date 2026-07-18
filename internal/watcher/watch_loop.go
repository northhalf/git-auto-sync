package watcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/rjeczalik/notify"
)

const (
	watchModeNormal = iota
	watchModeBackingOff
	watchModePaused
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

type watchDependencies struct {
	autoSync          func() error
	shouldIgnore      func(path string) (bool, error)
	isRemoteSyncError func(error) bool
	syncErrorStage    func(error) string
	retryDelays       []time.Duration
	// reportState receives state transitions; nil when the caller does not record state. It is
	// invoked on watcher start, on a successful sync, and on a non-remote error that pauses the
	// watcher. Remote-failure backoff does not invoke it because the watcher remains running.
	reportState func(StateReport)
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
// @param           fsLag         "delay applied while filesystem and wake events settle"
//
// @param           notifyEvents  "filesystem events, or nil when no filesystem source is attached"
//
// @param           awakeEvents   "platform wake events, or nil when wake notifications are unavailable"
//
// @param           syncTicks     "periodic synchronization ticks, or nil when periodic sync is disabled"
//
// @param           deps          "synchronization, classification, filtering, and retry dependencies"
//
// @return          returnErr  "nil on cancellation, or an event inspection error retained until the active sync finishes"
func runWatchLoop(
	ctx context.Context,
	logger *slog.Logger,
	fsLag time.Duration,
	notifyEvents <-chan notify.EventInfo,
	awakeEvents <-chan bool,
	syncTicks <-chan time.Time,
	deps watchDependencies,
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
			syncResults <- deps.autoSync()
		}()
	}

	requestDebouncedSync := func() {
		if syncRunning {
			pendingSync = true
			return
		}
		if fsLag <= 0 {
			startSync()
			return
		}
		stopTimer(debounceTimer)
		if debounceTimer == nil {
			debounceTimer = time.NewTimer(fsLag)
		} else {
			debounceTimer.Reset(fsLag)
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
		if deps.reportState != nil {
			deps.reportState(state)
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

			stage := deps.syncErrorStage(err)
			if deps.isRemoteSyncError(err) {
				mode = watchModeBackingOff
				delay := retryDelay(remoteFailures, deps.retryDelays)
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
			ignore := false
			var err error
			if deps.shouldIgnore != nil {
				ignore, err = deps.shouldIgnore(ei.Path())
			}
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
