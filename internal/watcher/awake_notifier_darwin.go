package watcher

import (
	"context"
	"log/slog"

	"github.com/prashantgupta24/mac-sleep-notifier/notifier"
)

// AwakeNotifierDarwin forwards Darwin suspend-and-resume wake events from the system IOKit
// notification source.
type AwakeNotifierDarwin struct {
	logger *slog.Logger
	n      *notifier.Notifier
}

// @description    Creates the Darwin wake notifier.
//
// NewAwakeNotifier creates a Darwin notifier backed by the system suspend-and-resume notification
// source.
//
// @param           logger  "repository-scoped logger used for startup diagnostics"
//
// @return          *AwakeNotifierDarwin  "notifier that reports system wake events"
//
// @return          error                "always nil"
func NewAwakeNotifier(logger *slog.Logger) (*AwakeNotifierDarwin, error) {
	n := notifier.GetInstance()

	return &AwakeNotifierDarwin{logger: logger, n: n}, nil
}

// @description    Forwards Darwin wake events until cancellation.
//
// Start begins listening for Darwin suspend-and-resume activity and forwards each wake event to the
// supplied channel. Its forwarding goroutine exits when ctx is canceled or the notifier channel
// closes.
//
// @param           ctx    "context whose cancellation stops event forwarding"
//
// @param           out    "channel that receives wake notifications"
//
// @return          error  "always nil after starting the forwarding goroutine"
func (a *AwakeNotifierDarwin) Start(ctx context.Context, out chan<- bool) error {
	suspendResumeNotifier := a.n.Start()
	if a.logger != nil {
		a.logger.Info("awake notifier started")
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case activity, ok := <-suspendResumeNotifier:
				if !ok {
					return
				}
				if activity.Type != notifier.Awake {
					continue
				}
				select {
				case out <- true:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return nil
}
