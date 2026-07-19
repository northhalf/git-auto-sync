//go:build linux && !android

package watcher

import (
	"context"
	"log/slog"

	"github.com/godbus/dbus/v5"
)

// logindBusName is the D-Bus service name of systemd-logind, the system daemon that broadcasts
// suspend and resume activity.
const logindBusName = "org.freedesktop.login1"

// logindManagerInterface is the D-Bus interface that carries the PrepareForSleep signal.
const logindManagerInterface = "org.freedesktop.login1.Manager"

// logindPrepareForSleep is the signal member systemd-logind emits before the system suspends
// (with a true argument) and after it resumes (with a false argument).
const logindPrepareForSleep = "PrepareForSleep"

// AwakeNotifierLinux forwards systemd-logind resume events received over the system D-Bus. When
// D-Bus or logind is unavailable (headless servers, containers, WSL), it degrades to a no-op that
// never forwards events, so the watcher keeps running on its periodic and filesystem triggers.
type AwakeNotifierLinux struct {
	logger *slog.Logger
	conn   *dbus.Conn
}

// @description    Creates the Linux wake notifier.
//
// NewAwakeNotifier connects to the system D-Bus and prepares to subscribe to logind's
// PrepareForSleep signal. When the system bus is unavailable the returned notifier is a no-op so
// the caller can keep starting the watcher without a hard failure.
//
// @param           logger  "repository-scoped logger used for degradation diagnostics"
//
// @return          *AwakeNotifierLinux  "notifier backed by the system D-Bus, or a no-op when the bus is unavailable"
//
// @return          error                "always nil; connection failures degrade to a no-op instead of an error"
func NewAwakeNotifier(logger *slog.Logger) (*AwakeNotifierLinux, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		if logger != nil {
			logger.Info("awake notifier disabled", "reason", "system dbus unavailable", "error", err)
		}
		return &AwakeNotifierLinux{logger: logger}, nil
	}
	return &AwakeNotifierLinux{logger: logger, conn: conn}, nil
}

// @description    Forwards Linux resume events until cancellation.
//
// Start subscribes to logind's PrepareForSleep signal and forwards each resume event (the signal's
// boolean argument is false) to the supplied channel. When the notifier has no D-Bus connection,
// or when signal subscription fails, Start returns nil without forwarding anything, so the watcher
// keeps running on its other triggers. Its forwarding goroutine exits when ctx is canceled, closing
// the D-Bus connection to release its resources.
//
// @param           ctx    "context whose cancellation stops event forwarding and closes the D-Bus connection"
//
// @param           out    "channel that receives wake notifications"
//
// @return          error  "always nil; subscription failures degrade to a no-op instead of an error"
func (a *AwakeNotifierLinux) Start(ctx context.Context, out chan<- bool) error {
	if a.conn == nil {
		return nil
	}
	if err := a.conn.AddMatchSignal(
		dbus.WithMatchSender(logindBusName),
		dbus.WithMatchInterface(logindManagerInterface),
		dbus.WithMatchMember(logindPrepareForSleep),
	); err != nil {
		if a.logger != nil {
			a.logger.Info("awake notifier disabled", "reason", "subscribe prepare-for-sleep failed", "error", err)
		}
		// A failed match rule leaves the connection usable for nothing here, so drop it.
		_ = a.conn.Close()
		a.conn = nil
		return nil
	}

	signals := make(chan *dbus.Signal, 16)
	a.conn.Signal(signals)

	go func() {
		defer func() { _ = a.conn.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case sig, ok := <-signals:
				if !ok {
					return
				}
				if len(sig.Body) == 0 {
					continue
				}
				resumed, ok := sig.Body[0].(bool)
				if !ok || resumed {
					// PrepareForSleep(true) is the pre-suspend signal; only forward the resume (false).
					continue
				}
				select {
				case out <- true:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return nil
}
