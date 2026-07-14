package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/kardianos/service"
	"github.com/northhalf/git-auto-sync/common"
	cfg "github.com/northhalf/git-auto-sync/common/config"
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
	config, err := cfg.Read()
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup

	for _, repoPath := range config.Repos {
		wg.Add(1)

		fmt.Println("Monitoring", repoPath)
		go watchForChanges(&wg, repoPath)
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
	daemon := Daemon{}
	autoSyncService, err := common.NewServiceWithDaemon(&daemon)
	if err != nil {
		log.Fatal("BuildService", err)
	}

	s := autoSyncService.Service
	logger, err := s.Logger(nil)
	if err != nil {
		log.Fatal("BuildLogger", err)
	}

	err = s.Run()
	if err != nil {
		_ = logger.Error("RunService", err)
	}
}

// FIXME: pass some kind of channel which tells this when to close!
// @description    Runs one repository watcher.
//
// watchForChanges builds repository configuration, runs the watcher, logs setup or watcher errors,
// and marks its wait-group task complete on return.
//
// @param           wg        "wait group tracking the repository watcher"
//
// @param           repoPath  "path to the repository to watch"
func watchForChanges(wg *sync.WaitGroup, repoPath string) {
	defer wg.Done()

	cfg, err := common.NewRepoConfig(repoPath)
	if err != nil {
		log.Println(err)
	}

	err = common.WatchForChanges(cfg)
	if err != nil {
		log.Println(err)
	}
}

// FIXME: Handle operating system signal which tells it to reload the config
