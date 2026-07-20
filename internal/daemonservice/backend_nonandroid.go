//go:build !android

package daemonservice

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"
)

// @description    Creates the native service-manager backend outside Android.
//
// @param           daemon  "service lifecycle implementation to register"
//
// @return          serviceBackend  "kardianos-backed service controller"
//
// @return          error           "nil on success, or an executable or service construction error"
func newServiceBackend(daemon service.Interface) (serviceBackend, error) {
	options := make(service.KeyValue)
	options["Restart"] = "on-success"
	options["UserService"] = true
	options["RunAtLoad"] = true

	ex, err := os.Executable()
	if err != nil {
		return nil, err
	}
	executablePath := filepath.Join(filepath.Dir(ex), daemonServiceName)
	if runtime.GOOS == "windows" {
		executablePath += ".exe"
	}

	deps := []string{}
	if runtime.GOOS == "linux" {
		deps = []string{"After=network-online.target syslog.target"}
	}

	config := &service.Config{
		Name:         daemonServiceName,
		DisplayName:  "Git Auto Sync Daemon",
		Description:  "Background Process for Auto Syncing Git Repos",
		Executable:   executablePath,
		Dependencies: deps,
		Option:       options,
	}
	if runtime.GOOS == "windows" {
		config.EnvVars = windowsUserEnvVars()
	}
	return service.New(daemon, config)
}
