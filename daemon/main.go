package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/kardianos/service"
	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/daemonservice"
	"github.com/northhalf/git-auto-sync/internal/logging"
)

type Daemon struct{}

// @description    Starts repository watchers.
//
// Start launches the daemon's repository watchers in a goroutine and returns immediately as
// required by the service interface.
//
// @param           s      "service instance requesting the daemon start"
//
// @return          error  "always nil after scheduling the daemon goroutine"
func (d *Daemon) Start(s service.Service) error {
	go d.run()
	return nil
}

// @description    Runs and reconciles repository watchers against the daemon configuration.
//
// run reads the daemon configuration, starts a watcher per repository, then polls the
// configuration file for changes and reconciles the running watchers: added repositories are
// started and removed repositories self-terminate. When the global synchronization settings
// (syncInterval, debounce, gitexec) change, every watcher is restarted so it picks up the new
// values. It panics if the initial configuration load fails and logs, rather than panics on,
// subsequent read errors.
func (d *Daemon) run() {
	mgr := newWatcherManager()

	daemonConfig, err := config.ReadGlobalSettings()
	if err != nil {
		panic(err)
	}
	mgr.reconcile(daemonConfig.Repos, daemonConfig.Envs)

	lastMod, err := config.GlobalSettingsModTime()
	if err != nil {
		panic(err)
	}
	lastSettings := daemonConfig

	ticker := time.NewTicker(configPollInterval)
	defer ticker.Stop()

	for range ticker.C {
		mod, err := config.GlobalSettingsModTime()
		if err != nil {
			slog.Error("read daemon config mtime failed", "error", err)
			continue
		}

		if !mod.Equal(lastMod) {
			lastMod = mod
			cur, err := config.ReadGlobalSettings()
			if err != nil {
				slog.Error("read daemon config failed", "error", err)
				continue
			}
			if settingsChanged(lastSettings, cur) {
				slog.Info("global settings changed, restarting watchers")
				mgr.RestartAll()
			}
			lastSettings = cur
			daemonConfig = cur
		}

		mgr.reconcile(daemonConfig.Repos, daemonConfig.Envs)
		mgr.Heartbeat()
	}
}

// @description    Stop returns immediately without stopping watcher goroutines.
//
// @param           s      "service instance requesting the daemon stop"
//
// @return          error  "always nil"
func (d *Daemon) Stop(s service.Service) error {
	// Stop should not block. Return with a few seconds.
	return nil
}

// @description    Runs the daemon service.
//
// main constructs the daemon service, obtains its logger, and runs the service. It terminates on
// setup errors and logs a service run error.
func main() {
	_, _ = logging.SetupDaemonLogger(os.Getenv("DEBUG") == "true")

	daemon := Daemon{}
	autoSyncService, err := daemonservice.NewServiceWithDaemon(&daemon)
	if err != nil {
		slog.Error("build service failed", "error", err)
		os.Exit(1)
	}

	s := autoSyncService.Service
	logger, err := s.Logger(nil)
	if err != nil {
		slog.Error("build service logger failed", "error", err)
		os.Exit(1)
	}

	err = s.Run()
	if err != nil {
		_ = logger.Error("RunService", err)
	}
}
