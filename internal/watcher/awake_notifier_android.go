//go:build android && arm64

package watcher

import (
	"context"
	"log/slog"
)

// AwakeNotifierAndroid is a no-op wake source for Android, where systemd-logind is unavailable.
type AwakeNotifierAndroid struct{}

// @description    Creates the Android no-op awake notifier.
//
// @param           logger  "unused repository logger retained for platform signature parity"
//
// @return          *AwakeNotifierAndroid  "no-op notifier that never emits wake events"
//
// @return          error                  "always nil"
func NewAwakeNotifier(logger *slog.Logger) (*AwakeNotifierAndroid, error) {
	return &AwakeNotifierAndroid{}, nil
}

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
