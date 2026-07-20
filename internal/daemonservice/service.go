package daemonservice

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/kardianos/service"
)

// ErrNotInstalled indicates the daemon service is not installed. Status returns it so callers can
// present a not-installed service as a normal status rather than a failed query.
var ErrNotInstalled = errors.New("the service is not installed")

type serviceBackend interface {
	Status() (service.Status, error)
	Install() error
	Uninstall() error
	Start() error
	Stop() error
}

type Service struct {
	Service serviceBackend
}

// isNotInstalled reports whether err is the kardianos not-installed error. kardianos signals a
// missing service via an error message rather than a sentinel, so the string check is centralized
// here for Status and EnsureRunning.
func isNotInstalled(err error) bool {
	return err != nil && strings.Contains(err.Error(), "the service is not installed")
}

// @description    Creates the daemon user service.
//
// NewServiceWithDaemon builds the user-service definition for the daemon executable beside the
// current binary, including platform service options.
//
// @param           daemon   "service lifecycle implementation to register"
//
// @return          Service  "configured service wrapper"
//
// @return          error    "nil on success, or an error resolving the executable or creating the service"
func NewServiceWithDaemon(daemon service.Interface) (Service, error) {
	backend, err := newServiceBackend(daemon)
	if err != nil {
		return Service{}, err
	}
	return Service{Service: backend}, nil
}

// @description    Collects the Windows user profile environment for the service process.
//
// windowsUserEnvVars returns APPDATA, LOCALAPPDATA, USERPROFILE, and Path from the current process
// so the daemon service, which runs as LocalSystem, resolves the same paths and executables as the
// installing user's CLI: APPDATA/LOCALAPPDATA for config, state, and log files; USERPROFILE so
// go-git and git resolve the user's global .gitconfig (author identity) and .ssh directory instead
// of LocalSystem's empty profile; and Path so the daemon finds git and its helper tools, which
// typically live in a user-PATH entry the LocalSystem process does not inherit. Path is captured as
// the already-merged, expanded Windows value, which is a superset of the system PATH, so overriding
// the service's inherited Path loses no system entries. Empty values are omitted so the service
// Environment registry value is not populated with blanks; when all are unset the returned map is
// empty and kardianos skips the registry write, leaving the daemon's inherited environment
// untouched. HOME is intentionally not injected because go-git and Git for Windows resolve the home
// directory from USERPROFILE on Windows, and a MSYS-style HOME captured from a git-bash CLI would be
// a non-Windows path.
//
// @return          map[string]string  "APPDATA, LOCALAPPDATA, USERPROFILE, and Path entries present only when set"
func windowsUserEnvVars() map[string]string {
	envVars := map[string]string{}
	if v := os.Getenv("APPDATA"); v != "" {
		envVars["APPDATA"] = v
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		envVars["LOCALAPPDATA"] = v
	}
	if v := os.Getenv("USERPROFILE"); v != "" {
		envVars["USERPROFILE"] = v
	}
	if v := os.Getenv("Path"); v != "" {
		envVars["Path"] = v
	}
	return envVars
}

// @description    Starts the daemon service only when it is not already running.
//
// EnsureRunning starts the daemon when it is installed but stopped, and installs and starts it
// when it is not installed. When installation reports an existing init entry, it attempts an
// uninstall and reinstall but ignores errors from those recovery operations. When the service is
// already running, it does nothing so that a running daemon picks up configuration changes through
// its reload poller instead of being restarted.
//
// @return          error  "nil on success or when already running, or an error querying, installing, or starting the service"
func (srv Service) EnsureRunning() error {
	status, err := srv.Service.Status()
	if err != nil {
		if !isNotInstalled(err) {
			return err
		}
		// Not installed: install and start.
		if err := srv.Service.Install(); err != nil {
			if strings.Contains(err.Error(), "Init already exists") {
				slog.Info("service init entry already exists; reinstalling git-auto-sync-daemon")
				_ = srv.Service.Uninstall()
				_ = srv.Service.Install()
			} else {
				return err
			}
		} else {
			logStep("Installing git-auto-sync as a daemon")
		}
		logStep("Starting git-auto-sync-daemon")
		return srv.Service.Start()
	}

	if status == service.StatusRunning {
		return nil
	}

	logStep("Starting git-auto-sync-daemon")
	return srv.Service.Start()
}

// @description    Stops the daemon service without uninstalling it.
//
// Stop stops a running daemon service so it can be started again later without reinstalling. It
// announces the stop step to the user and the CLI log. Unlike Disable, it leaves the service
// installed.
//
// @return          error  "nil on success, or a wrapped error from the stop operation"
func (srv Service) Stop() error {
	logStep("Stopping git-auto-sync-daemon")
	if err := srv.Service.Stop(); err != nil {
		return err
	}

	return nil
}

// @description    Restarts the daemon service.
//
// Restart stops a running daemon service and starts it again. When the service is already stopped
// it is started without a preceding stop. A not-installed service is reported as ErrNotInstalled so
// the caller can suggest installation rather than attempting a restart.
//
// @return          error  "nil on success, ErrNotInstalled when not installed, or an error querying, stopping, or starting the service"
func (srv Service) Restart() error {
	status, err := srv.Status()
	if err != nil {
		return err
	}

	if status == service.StatusRunning {
		if err := srv.Stop(); err != nil {
			return err
		}
	}

	logStep("Restarting git-auto-sync-daemon")
	return srv.Service.Start()
}

// @description    Disable stops and uninstalls the daemon user service.
//
// Disable is idempotent: it queries the service status first and only stops a running service, so
// calling it on an already-stopped service does not surface a stop error. A not-installed service is
// treated as already disabled and returns nil, so removing the last monitored repository or running
// the uninstall command on a service that was already removed both succeed.
//
// @return          error  "nil on success or when not installed, or an error stopping or uninstalling the service"
func (srv Service) Disable() error {
	status, err := srv.Status()
	if err != nil {
		if errors.Is(err, ErrNotInstalled) {
			return nil
		}
		return err
	}

	if status == service.StatusRunning {
		logStep("Stopping git-auto-sync-daemon")
		if err := srv.Service.Stop(); err != nil {
			return err
		}
	}

	logStep("Uninstalling git-auto-sync as a daemon")
	if err := srv.Service.Uninstall(); err != nil {
		return err
	}

	return nil
}

// @description    Returns the daemon service status.
//
// Status queries the daemon service and returns its status without printing. A not-installed
// service is reported as ErrNotInstalled so callers can present it as a normal status rather than
// inspecting the kardianos error string or treating it as a failed query.
//
// @return          service.Status  "current service status, or StatusUnknown when the query fails"
//
// @return          error            "ErrNotInstalled when the service is not installed, a wrapped error for other query failures, or nil on success"
func (srv Service) Status() (service.Status, error) {
	status, err := srv.Service.Status()
	if err != nil {
		if isNotInstalled(err) {
			return service.StatusUnknown, ErrNotInstalled
		}
		return service.StatusUnknown, err
	}
	return status, nil
}

// @description    Prints a service-lifecycle message to stdout and the CLI log.
//
// logStep announces a service operation to the user on stdout and records the same message in the
// CLI log file via slog so install, start, stop, and uninstall actions are auditable alongside
// other CLI events.
//
// @param           msg  "human-readable description of the service step"
func logStep(msg string) {
	fmt.Println(msg)
	slog.Info(msg)
}
