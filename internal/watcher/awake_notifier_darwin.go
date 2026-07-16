package watcher

import (
	"context"

	"github.com/prashantgupta24/mac-sleep-notifier/notifier"
)

type AwakeNotifierDarwn struct {
	n *notifier.Notifier
}

// @description    Creates the Darwin wake notifier.
//
// NewAwakeNotifier creates a Darwin notifier backed by the system suspend-and-resume notification
// source.
//
// @return          *AwakeNotifierDarwn  "notifier that reports system wake events"
//
// @return          error                "always nil"
func NewAwakeNotifier() (*AwakeNotifierDarwn, error) {
	n := notifier.GetInstance()

	return &AwakeNotifierDarwn{n: n}, nil
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
func (a *AwakeNotifierDarwn) Start(ctx context.Context, out chan<- bool) error {
	suspendResumeNotifier := a.n.Start()

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
