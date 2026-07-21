//go:build android && arm64

package main

import (
	"log/slog"
	"os"

	"github.com/northhalf/git-auto-sync/internal/logging"
	"github.com/northhalf/git-auto-sync/internal/notification"
	"github.com/northhalf/git-auto-sync/internal/termux"
)

// @description    Runs the Android daemon in the foreground for runit supervision.
func main() {
	termux.ApplyLocalTimezone()
	logging.SetupDaemonLogger(os.Getenv("DEBUG") == "true")
	slog.Info("Start git-auto-sync daemon")
	notification.WarnIfUnavailable(slog.Default())

	daemon := Daemon{}
	daemon.run()
}
