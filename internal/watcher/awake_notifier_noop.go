package watcher

import "context"

// AwakeNotifierAndroid is a no-op wake source for Android, where systemd-logind is unavailable.
type AwakeNotifierAndroid struct{}

// @description    Leaves Android wake notifications disabled.
//
// @param           ctx  "unused watcher context"
//
// @param           out  "unused wake event channel"
//
// @return          error  "always nil"
func (a *AwakeNotifierAndroid) Start(ctx context.Context, out chan<- bool) error {
	return nil
}
