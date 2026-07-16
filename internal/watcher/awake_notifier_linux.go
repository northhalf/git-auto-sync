package watcher

import "context"

type AwakeNotifierEmpty struct{}

// @description    Start performs no work because Linux wake notifications are not implemented.
//
// @param           ctx    "context that would stop wake notification forwarding"
//
// @param           out    "channel that would receive wake notifications"
//
// @return          error  "always nil"
func (AwakeNotifierEmpty) Start(context.Context, chan<- bool) error {
	return nil
}

// @description    NewAwakeNotifier creates the Linux no-op wake notifier.
//
// @return          AwakeNotifier  "no-op notifier used on Linux"
//
// @return          error          "always nil"
func NewAwakeNotifier() (AwakeNotifier, error) {
	return AwakeNotifierEmpty{}, nil
}
