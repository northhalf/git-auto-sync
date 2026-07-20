//go:build !android

package notification

import (
	_ "embed"
	"log/slog"

	"github.com/gen2brain/beeep"
)

// warningPNG contains the embedded rebase-conflict notification icon.
//
//go:embed warning.png
var warningPNG []byte

type desktopNotifier struct{}

// @description    Creates the desktop notification backend used outside Android.
//
// @return          notifier  "beeep-backed platform notifier"
func newPlatformNotifier() notifier {
	return desktopNotifier{}
}

// @description    Sends a desktop alert through beeep.
//
// @param           title    "desktop notification title"
//
// @param           content  "desktop notification body"
//
// @return          error    "nil on success, or an error from beeep"
func (desktopNotifier) Alert(title, content string) error {
	return beeep.Alert(title, content, warningPNG)
}

// @description    Performs no availability warning on desktop platforms.
//
// @param           logger  "unused logger retained for notifier interface parity"
func (desktopNotifier) WarnIfUnavailable(logger *slog.Logger) {}
