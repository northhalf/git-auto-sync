//go:build android && arm64

package notification

import (
	"context"
	"os"
	"os/exec"
	"time"
)

const termuxNotificationTimeout = 10 * time.Second

// @description    Creates the Android notification backend.
//
// @return          notifier  "termux-notification-backed platform notifier"
func newPlatformNotifier() notifier {
	return newTermuxNotifier(
		func() (string, error) {
			return findTermuxNotification(os.Getenv, exec.LookPath)
		},
		runNotificationCommand,
		os.Stderr,
		termuxNotificationTimeout,
	)
}

// @description    Executes termux-notification and captures its combined output.
//
// @param           ctx   "context that limits command execution time"
//
// @param           path  "absolute or PATH-resolved command path"
//
// @param           args  "notification command arguments"
//
// @return          []byte  "combined standard output and standard error"
//
// @return          error   "nil on success, or the command execution error"
func runNotificationCommand(ctx context.Context, path string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}
