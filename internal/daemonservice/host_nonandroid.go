//go:build !android

package daemonservice

import (
	"fmt"

	"github.com/kardianos/service"
)

// @description    Runs the daemon under the native non-Android service manager.
//
// RunHostedDaemon creates the same service definition used by CLI lifecycle commands, opens its
// platform logger, and blocks in kardianos/service until the service manager stops the process.
// A service Run error is written through the native service logger and treated as handled, matching
// the previous daemon entry-point behavior.
//
// @param           daemon  "daemon lifecycle implementation to host"
//
// @return          error   "service construction or logger setup error"
func RunHostedDaemon(daemon service.Interface) error {
	backend, err := newServiceBackend(daemon)
	if err != nil {
		return err
	}
	host, ok := backend.(service.Service)
	if !ok {
		return fmt.Errorf("native service backend cannot host the daemon")
	}
	logger, err := host.Logger(nil)
	if err != nil {
		return err
	}
	if err := host.Run(); err != nil {
		_ = logger.Error("RunService", err)
	}
	return nil
}
