package watcher

import "github.com/prashantgupta24/mac-sleep-notifier/notifier"

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

// @description    Forwards Darwin wake events.
//
// Start begins listening for Darwin suspend-and-resume activity and forwards each wake event to
// the supplied channel from a goroutine.
//
// @param           out    "channel that receives wake notifications"
//
// @return          error  "always nil after starting the forwarding goroutine"
func (a *AwakeNotifierDarwn) Start(out chan bool) error {
	suspendResumeNotifier := a.n.Start()

	go func() {
		for {
			activity := <-suspendResumeNotifier
			if activity.Type == notifier.Awake {
				out <- true
			}
		}
	}()

	return nil
}
