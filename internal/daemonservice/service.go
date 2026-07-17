package daemonservice

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kardianos/service"
	"github.com/ztrue/tracerr"
)

// ErrNotInstalled indicates the daemon service is not installed. Status returns it so callers can
// present a not-installed service as a normal status rather than a failed query.
var ErrNotInstalled = errors.New("the service is not installed")

type Service struct {
	Service service.Service
}

// isNotInstalled reports whether err is the kardianos not-installed error. kardianos signals a
// missing service via an error message rather than a sentinel, so the string check is centralized
// here for Status, Enable, and EnsureRunning.
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
	options := make(service.KeyValue)
	options["Restart"] = "on-success"
	options["UserService"] = true
	options["RunAtLoad"] = true

	ex, err := os.Executable()
	if err != nil {
		return Service{}, tracerr.Wrap(err)
	}
	exDirPath := filepath.Dir(ex)
	executablePath := filepath.Join(exDirPath, "git-auto-sync-daemon")

	deps := []string{}
	if runtime.GOOS == "linux" {
		deps = []string{"After=network-online.target syslog.target"}
	}

	svcConfig := &service.Config{
		Name:        "git-auto-sync-daemon",
		DisplayName: "Git Auto Sync Daemon",
		Description: "Background Process for Auto Syncing Git Repos",

		Executable:   executablePath,
		Dependencies: deps,
		Option:       options,
	}

	s, err := service.New(daemon, svcConfig)
	if err != nil {
		return Service{}, tracerr.Wrap(err)
	}

	return Service{Service: s}, nil
}

// @description    Installs and starts the daemon service.
//
// Enable stops a running daemon service, installs it, and starts it. When installation reports an
// existing init entry, it attempts an uninstall and reinstall but ignores errors from those
// recovery operations.
//
// @return          error  "nil on completion, or a status, stop, non-recoverable install, or start error"
func (srv Service) Enable() error {
	s := srv.Service

	status, err := s.Status()
	if err != nil {
		if !isNotInstalled(err) {
			return tracerr.Wrap(err)
		}
	}

	stopped := false
	if status == service.StatusRunning {
		err := s.Stop()
		if err != nil {
			return tracerr.Wrap(err)
		}
		stopped = true
	}

	err = s.Install()
	if err != nil {
		if strings.Contains(err.Error(), "Init already exists") {
			slog.Info("service init entry already exists; reinstalling git-auto-sync-daemon")
			_ = s.Uninstall()
			_ = s.Install()
		} else {
			return tracerr.Wrap(err)
		}
	} else {
		logStep("Installing git-auto-sync as a daemon")
	}

	if stopped {
		logStep("Restarting git-auto-sync-daemon")
	} else {
		logStep("Starting git-auto-sync-daemon")
	}

	err = s.Start()
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}

// @description    Starts the daemon service only when it is not already running.
//
// EnsureRunning starts the daemon when it is installed but stopped, and installs and starts it
// when it is not installed. When the service is already running, it does nothing so that a
// running daemon picks up configuration changes through its reload poller instead of being
// restarted.
//
// @return          error  "nil on success or when already running, or an error querying, installing, or starting the service"
func (srv Service) EnsureRunning() error {
	status, err := srv.Service.Status()
	if err != nil {
		if !isNotInstalled(err) {
			return tracerr.Wrap(err)
		}
		// Not installed: install and start via Enable, which stops nothing here.
		return srv.Enable()
	}

	if status == service.StatusRunning {
		return nil
	}

	logStep("Starting git-auto-sync-daemon")
	return tracerr.Wrap(srv.Service.Start())
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
		return tracerr.Wrap(err)
	}

	return nil
}

// @description    Disable stops and uninstalls the daemon user service.
//
// @return          error  "nil on success, or an error stopping or uninstalling the service"
func (srv Service) Disable() error {
	logStep("Stopping git-auto-sync-daemon")
	err := srv.Service.Stop()
	if err != nil {
		return tracerr.Wrap(err)
	}

	logStep("Uninstalling git-auto-sync as a daemon")
	err = srv.Service.Uninstall()
	if err != nil {
		return tracerr.Wrap(err)
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
		return service.StatusUnknown, tracerr.Wrap(err)
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
