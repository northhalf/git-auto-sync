package notification

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"time"
)

const termuxUnavailableWarning = "warning: Android notifications are unavailable because termux-notification was not found.\nInstall the Termux:API Android app, then run:\npkg install termux-api\n"

// ErrUnavailable indicates that the platform notification command is not installed.
var ErrUnavailable = errors.New("notifications unavailable")

type executableFinder func() (string, error)

type notificationRunner func(context.Context, string, ...string) ([]byte, error)

type termuxNotifier struct {
	finder  executableFinder
	runner  notificationRunner
	stderr  io.Writer
	timeout time.Duration

	resolveOnce sync.Once
	path        string
	resolveErr  error
	warnOnce    sync.Once
}

// @description    Creates a Termux notification backend with injectable platform dependencies.
//
// @param           finder   "resolver for the termux-notification executable"
//
// @param           runner   "command runner used to send a notification"
//
// @param           stderr   "writer for startup availability warnings"
//
// @param           timeout  "maximum duration allowed for a notification command"
//
// @return          *termuxNotifier  "configured Termux notification backend"
func newTermuxNotifier(finder executableFinder, runner notificationRunner, stderr io.Writer, timeout time.Duration) *termuxNotifier {
	return &termuxNotifier{finder: finder, runner: runner, stderr: stderr, timeout: timeout}
}

// @description    Finds termux-notification under PREFIX before falling back to PATH.
//
// @param           getenv    "environment lookup used to read PREFIX"
//
// @param           lookPath  "executable resolver used for absolute and PATH lookups"
//
// @return          string    "resolved notification command path"
//
// @return          error     "lookup error when the command cannot be found"
func findTermuxNotification(getenv func(string) string, lookPath func(string) (string, error)) (string, error) {
	if prefix := getenv("PREFIX"); prefix != "" {
		candidate := filepath.Join(prefix, "bin", "termux-notification")
		if path, err := lookPath(candidate); err == nil {
			return path, nil
		}
	}
	return lookPath("termux-notification")
}

// @description    Resolves and caches the Termux notification executable.
//
// @return          string  "absolute or PATH-resolved notification executable"
//
// @return          error   "resolution failure when the command is unavailable"
func (n *termuxNotifier) resolve() (string, error) {
	n.resolveOnce.Do(func() {
		n.path, n.resolveErr = n.finder()
		if n.resolveErr != nil {
			n.resolveErr = fmt.Errorf("%w: %v", ErrUnavailable, n.resolveErr)
		}
	})
	return n.path, n.resolveErr
}

// @description    Warns once when Android notifications are unavailable.
//
// @param           logger  "logger that receives the non-fatal availability warning"
func (n *termuxNotifier) WarnIfUnavailable(logger *slog.Logger) {
	if _, err := n.resolve(); err == nil {
		return
	}

	n.warnOnce.Do(func() {
		_, _ = fmt.Fprint(n.stderr, termuxUnavailableWarning)
		if logger != nil {
			logger.Warn("Android notifications are unavailable", "reason", "termux-notification was not found", "install", "pkg install termux-api")
		}
	})
}

// @description    Sends a notification through the Termux:API command.
//
// @param           title    "Android notification title"
//
// @param           content  "Android notification body"
//
// @return          error    "nil on success, or an executable resolution or command error"
func (n *termuxNotifier) Alert(title, content string) error {
	path, err := n.resolve()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	output, err := n.runner(ctx, path, "--title", title, "--content", content)
	if err != nil {
		return fmt.Errorf("termux-notification failed: %w: %s", err, output)
	}
	return nil
}
