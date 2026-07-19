package notification

import "log/slog"

type notifier interface {
	Alert(title, content string) error
	WarnIfUnavailable(logger *slog.Logger)
}

var defaultNotifier = newPlatformNotifier()

// @description    Sends a platform-native notification.
//
// @param           title    "notification title"
//
// @param           content  "notification body"
//
// @return          error    "nil on success, or a platform notification error"
func Alert(title, content string) error {
	return defaultNotifier.Alert(title, content)
}

// @description    Warns when an optional platform notification dependency is unavailable.
//
// @param           logger  "logger that receives the non-fatal availability warning"
func WarnIfUnavailable(logger *slog.Logger) {
	defaultNotifier.WarnIfUnavailable(logger)
}
