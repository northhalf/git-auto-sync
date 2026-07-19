package main

import (
	"log/slog"

	"github.com/northhalf/git-auto-sync/internal/notification"
	"github.com/urfave/cli/v2"
)

var warnNotificationUnavailable = notification.WarnIfUnavailable

// @description    Checks optional platform notification dependencies before sync-capable commands.
//
// @param           ctx    "CLI context supplied by urfave/cli"
//
// @return          error  "always nil because notification support is optional"
func notificationAvailabilityHook(ctx *cli.Context) error {
	warnNotificationUnavailable(slog.Default())
	return nil
}
