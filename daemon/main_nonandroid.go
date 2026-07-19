//go:build !android

package main

import (
	"log/slog"
	"os"

	"github.com/northhalf/git-auto-sync/internal/daemonservice"
	"github.com/northhalf/git-auto-sync/internal/logging"
)

// @description    Runs the daemon through the native non-Android service manager.
func main() {
	_, _ = logging.SetupDaemonLogger(os.Getenv("DEBUG") == "true")
	slog.Info("Start git-auto-sync daemon")

	if err := daemonservice.RunHostedDaemon(&Daemon{}); err != nil {
		slog.Error("run daemon service failed", "error", err)
		os.Exit(1)
	}
}
