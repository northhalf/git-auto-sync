package watcher

type AwakeNotifierEmpty struct{}

// @description    Start performs no work because Linux wake notifications are not implemented.
//
// @param           out    "channel that would receive wake notifications"
//
// @return          error  "always nil"
func (AwakeNotifierEmpty) Start(chan bool) error {
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
