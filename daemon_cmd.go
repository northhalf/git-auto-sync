package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kardianos/service"
	cfg "github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/daemonservice"
	"github.com/urfave/cli/v2"
	"github.com/ztrue/tracerr"
)

// cliDaemon satisfies the service interface for CLI service management without starting a process.
type cliDaemon struct{}

// @description    Satisfies the service interface without starting a daemon process.
//
// Start is a no-op used by CLI service management, which builds the service definition without
// running a daemon process.
//
// @param           service.Service  "service instance requesting the daemon start"
//
// @return          error            "always nil"
func (cliDaemon) Start(service.Service) error { return nil }

// @description    Satisfies the service interface without stopping a daemon process.
//
// Stop is a no-op used by CLI service management, which never starts a daemon process to stop.
//
// @param           service.Service  "service instance requesting the daemon stop"
//
// @return          error            "always nil"
func (cliDaemon) Stop(service.Service) error { return nil }

// @description    Prints daemon status and repositories.
//
// daemonStatus queries the service status, prints a message for running, stopped, and not-installed
// states, then prints every repository in the daemon configuration. A not-installed service is
// reported as a normal status; other service or configuration errors are returned for the CLI to
// surface.
//
// @param           ctx    "CLI context for the daemon status command"
//
// @return          error  "nil on success, or an error from the service or configuration"
func daemonStatus(ctx *cli.Context) error {
	s, err := daemonservice.NewServiceWithDaemon(cliDaemon{})
	if err != nil {
		return tracerr.Wrap(err)
	}

	status, err := s.Status()
	switch {
	case errors.Is(err, daemonservice.ErrNotInstalled):
		fmt.Println("git-auto-sync-daemon is NOT installed!")
	case err != nil:
		return tracerr.Wrap(err)
	case status == service.StatusRunning:
		fmt.Println("git-auto-sync-daemon is Running!")
	case status == service.StatusStopped:
		fmt.Println("git-auto-sync-daemon is NOT Running!")
	case status == service.StatusUnknown:
		// No user-facing message for an unknown status.
	default:
		fmt.Println("git-auto-sync-daemon status is Unknown. How mysterious!")
	}

	settings, err := cfg.ReadGlobalSettings()
	if err != nil {
		return tracerr.Wrap(err)
	}

	fmt.Println("Monitoring - ")
	for _, repoPath := range settings.Repos {
		fmt.Println("  ", repoPath)
	}

	// FIXME: Print out if there are any 'rebasing' issues and we are paused

	return nil
}

// @description    Starts the daemon service, installing it when missing.
//
// daemonRun ensures the daemon service is installed and running, then reports the outcome. When the
// service is not installed, EnsureRunning installs it first; when it is stopped, it is started; when
// it is already running, it is left untouched. After the start attempt, daemonRun prints whether the
// daemon started, was already running, or failed to start.
//
// @param           ctx    "CLI context for the daemon run command"
//
// @return          error  "nil on success, or an error building, querying, installing, or starting the service"
func daemonRun(ctx *cli.Context) error {
	s, err := daemonservice.NewServiceWithDaemon(cliDaemon{})
	if err != nil {
		return tracerr.Wrap(err)
	}

	alreadyRunning := false
	if status, queryErr := s.Status(); queryErr == nil {
		alreadyRunning = status == service.StatusRunning
	} else if !errors.Is(queryErr, daemonservice.ErrNotInstalled) {
		return tracerr.Wrap(queryErr)
	}

	if err := s.EnsureRunning(); err != nil {
		fmt.Println("git-auto-sync-daemon failed to start")
		return tracerr.Wrap(err)
	}

	if alreadyRunning {
		fmt.Println("git-auto-sync-daemon is already running")
	} else {
		fmt.Println("git-auto-sync-daemon started successfully")
	}

	return nil
}

// @description    Stops the daemon service.
//
// daemonStop stops a running daemon service and reports the outcome. When the service is not
// installed or already stopped, it reports that state without attempting a stop. After a stop
// attempt it prints whether the daemon stopped or failed to stop.
//
// @param           ctx    "CLI context for the daemon stop command"
//
// @return          error  "nil on success, or an error building, querying, or stopping the service"
func daemonStop(ctx *cli.Context) error {
	s, err := daemonservice.NewServiceWithDaemon(cliDaemon{})
	if err != nil {
		return tracerr.Wrap(err)
	}

	status, queryErr := s.Status()
	if queryErr != nil {
		if errors.Is(queryErr, daemonservice.ErrNotInstalled) {
			fmt.Println("git-auto-sync-daemon is NOT installed")
			return nil
		}
		return tracerr.Wrap(queryErr)
	}

	if status != service.StatusRunning {
		fmt.Println("git-auto-sync-daemon is not running")
		return nil
	}

	if err := s.Stop(); err != nil {
		fmt.Println("git-auto-sync-daemon failed to stop")
		return tracerr.Wrap(err)
	}

	fmt.Println("git-auto-sync-daemon stopped successfully")

	return nil
}

// @description    daemonList prints each repository stored in the daemon configuration.
//
// @param           ctx    "CLI context for the daemon list command"
//
// @return          error  "nil on success, or an error reading the configuration"
func daemonList(ctx *cli.Context) error {
	settings, err := cfg.ReadGlobalSettings()
	if err != nil {
		return tracerr.Wrap(err)
	}

	for _, repoPath := range settings.Repos {
		fmt.Println(repoPath)
	}
	return nil
}

// @description    Adds a repository to the daemon.
//
// daemonAdd validates a repository, adds it to the daemon configuration when absent, writes the
// configuration, and ensures the daemon service is running. A running daemon picks up the new
// repository through its configuration reload poller; a stopped or uninstalled daemon is started
// or installed.
//
// @param           ctx    "CLI context containing the repository path"
//
// @return          error  "nil on success, or an error validating, persisting, or starting the service"
func daemonAdd(ctx *cli.Context) error {
	repoPath := ctx.Args().First()
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return tracerr.Wrap(err)
	}

	repoPath, err = isValidGitRepo(repoPath)
	if err != nil {
		return tracerr.Wrap(err)
	}

	settings, err := cfg.ReadGlobalSettings()
	if err != nil {
		return tracerr.Wrap(err)
	}

	if slices.Contains(settings.Repos, repoPath) {
		fmt.Println("The Daemon is already monitoring " + repoPath)
	} else {
		settings.Repos = append(settings.Repos, repoPath)
	}

	err = cfg.WriteGlobalSettings(settings)
	if err != nil {
		return tracerr.Wrap(err)
	}

	s, err := daemonservice.NewServiceWithDaemon(cliDaemon{})
	if err != nil {
		return tracerr.Wrap(err)
	}

	err = s.EnsureRunning()
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}

// @description    Removes a repository from the daemon.
//
// daemonRm validates and removes a tracked repository from the daemon configuration, then stops
// and uninstalls the service when no repositories remain.
//
// @param           ctx    "CLI context containing the repository path"
//
// @return          error  "nil on success, or an error validating, persisting, or disabling the service"
func daemonRm(ctx *cli.Context) error {
	repoPath := ctx.Args().First()
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return tracerr.Wrap(err)
	}

	repoPath, err = isValidGitRepo(repoPath)
	if err != nil {
		return tracerr.Wrap(err)
	}

	settings, err := cfg.ReadGlobalSettings()
	if err != nil {
		return tracerr.Wrap(err)
	}

	pos := -1
	for i, rp := range settings.Repos {
		if rp == repoPath {
			pos = i
			break
		}
	}

	if pos == -1 {
		err = errors.New("repo not tracked")
		return tracerr.Errorf("%w - %s", err, repoPath)
	}

	settings.Repos = append(settings.Repos[:pos], settings.Repos[pos+1:]...)
	err = cfg.WriteGlobalSettings(settings)
	if err != nil {
		return tracerr.Wrap(err)
	}

	if len(settings.Repos) == 0 {
		s, err := daemonservice.NewServiceWithDaemon(cliDaemon{})
		if err != nil {
			return tracerr.Wrap(err)
		}

		err = s.Disable()
		if err != nil {
			return tracerr.Wrap(err)
		}
	}

	return nil
}

// @description    Updates the daemon environment.
//
// daemonEnv validates key=value arguments, merges them into the daemon environment configuration,
// persists the result, and prints all stored entries. Invalid argument syntax terminates the
// process through the logger.
//
// @param           ctx    "CLI context containing environment assignments"
//
// @return          error  "nil on success, or an error reading or writing the configuration"
func daemonEnv(ctx *cli.Context) error {
	vars := ctx.Args().Slice()

	for _, v := range vars {
		if !strings.Contains(v, "=") {
			log.Fatalln("Env variables must be in the format 'key=value'")
		}
	}

	settings, err := cfg.ReadGlobalSettings()
	if err != nil {
		return tracerr.Wrap(err)
	}

	envMap := make(map[string]string, len(settings.Envs)+len(vars))
	for _, e := range settings.Envs {
		k, v, _ := strings.Cut(e, "=")
		envMap[k] = v
	}
	for _, v := range vars {
		k, val, _ := strings.Cut(v, "=")
		envMap[k] = val
	}

	settings.Envs = make([]string, 0, len(envMap))
	for k, v := range envMap {
		settings.Envs = append(settings.Envs, k+"="+v)
	}
	err = cfg.WriteGlobalSettings(settings)
	if err != nil {
		return tracerr.Wrap(err)
	}

	fmt.Println(strings.Join(settings.Envs, "\n"))

	return nil
}
