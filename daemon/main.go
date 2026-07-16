package main

import (
	"log/slog"
	"os"
	"sync"

	"github.com/kardianos/service"
	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/daemonservice"
	"github.com/northhalf/git-auto-sync/internal/logging"
	"github.com/northhalf/git-auto-sync/internal/watcher"
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

// @description    Runs all configured repository watchers.
//
// run reads the daemon configuration, starts one watcher goroutine per repository, and blocks
// until all watchers stop. It panics if configuration loading fails.
func (d *Daemon) run() {
	daemonConfig, err := config.ReadDaemonConfig()
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup

	for _, repoPath := range daemonConfig.Repos {
		wg.Add(1)

		logger := logging.WithRepo(repoPath)
		logger.Info("monitoring repo")
		go watchForChanges(&wg, logger, repoPath, daemonConfig.Envs)
	}

	wg.Wait()
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

// FIXME: pass some kind of channel which tells this when to close!
// @description    Runs one repository watcher.
//
// watchForChanges builds repository configuration, applies the daemon's environment entries, runs
// the watcher, logs setup or watcher errors, and marks its wait-group task complete on return.
//
// @param           wg        "wait group tracking the repository watcher"
//
// @param           logger    "repository-scoped logger"
//
// @param           repoPath  "path to the repository to watch"
//
// @param           env       "daemon-level environment entries applied to the repository configuration"
func watchForChanges(wg *sync.WaitGroup, logger *slog.Logger, repoPath string, env []string) {
	defer wg.Done()

	cfg, err := config.NewRepoConfig(repoPath)
	if err != nil {
		logger.Error("build repo config failed", "error", err)
		return
	}
	cfg.Env = append(cfg.Env, env...)

	_ = watcher.WatchForChanges(logger, cfg)
}

// FIXME: Handle operating system signal which tells it to reload the config
