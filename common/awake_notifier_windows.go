package common

type AwakeNotifierWindows struct{}

// @description    Start performs no work because Windows wake notifications are not implemented.
//
// @param           out    "channel that would receive wake notifications"
//
// @return          error  "always nil"
func (AwakeNotifierWindows) Start(chan bool) error {
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
