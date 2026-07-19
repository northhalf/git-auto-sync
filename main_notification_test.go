package main

import (
	"log/slog"
	"testing"

	"github.com/urfave/cli/v2"
)

// @description    Verifies the notification availability hook invokes the platform warning check.
//
// @param           t  "test handle used for assertions"
func TestNotificationAvailabilityHookChecksPlatformNotifier(t *testing.T) {
	previous := warnNotificationUnavailable
	called := false
	warnNotificationUnavailable = func(*slog.Logger) { called = true }
	t.Cleanup(func() { warnNotificationUnavailable = previous })

	if err := notificationAvailabilityHook(&cli.Context{}); err != nil {
		t.Fatalf("notificationAvailabilityHook returned error %v, want nil", err)
	}
	if !called {
		t.Fatal("notificationAvailabilityHook did not check notifier availability")
	}
}
