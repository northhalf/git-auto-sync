package watcher

import "context"

type AwakeNotifierWindows struct{}

// @description    Start performs no work because Windows wake notifications are not implemented.
//
// @param           ctx    "context that would stop wake notification forwarding"
//
// @param           out    "channel that would receive wake notifications"
//
// @return          error  "always nil"
func (AwakeNotifierWindows) Start(context.Context, chan<- bool) error {
	return nil
}

// @description    NewAwakeNotifier creates the Windows no-op wake notifier.
//
// @return          AwakeNotifier  "no-op notifier used on Windows"
//
// @return          error          "always nil"
func NewAwakeNotifier() (AwakeNotifier, error) {
	return AwakeNotifierWindows{}, nil
}
