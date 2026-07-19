//go:build android && arm64

package watcher

import "log/slog"

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
