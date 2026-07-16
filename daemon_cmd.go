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

// Start satisfies the service interface without starting a daemon process.
func (cliDaemon) Start(service.Service) error { return nil }

// Stop satisfies the service interface without stopping a daemon process.
func (cliDaemon) Stop(service.Service) error { return nil }

// @description    Prints daemon status and repositories.
//
// daemonStatus prints the service status and every repository in the daemon configuration,
// returning an error if either cannot be read.
//
// @param           ctx    "CLI context for the daemon status command"
//
// @return          error  "nil on success, or an error from the service or configuration"
func daemonStatus(ctx *cli.Context) error {
	s, err := daemonservice.NewServiceWithDaemon(cliDaemon{})
	if err != nil {
		return tracerr.Wrap(err)
	}

	err = s.Status()
	if err != nil {
		return tracerr.Wrap(err)
	}

	config, err := cfg.ReadDaemonConfig()
	if err != nil {
		return tracerr.Wrap(err)
	}

	fmt.Println("Monitoring - ")
	for _, repoPath := range config.Repos {
		fmt.Println("  ", repoPath)
	}

	// FIXME: Print out if there are any 'rebasing' issues and we are paused

	return nil
}

// @description    daemonList prints each repository stored in the daemon configuration.
//
// @param           ctx    "CLI context for the daemon list command"
//
// @return          error  "nil on success, or an error reading the configuration"
func daemonList(ctx *cli.Context) error {
	config, err := cfg.ReadDaemonConfig()
	if err != nil {
		return tracerr.Wrap(err)
	}

	for _, repoPath := range config.Repos {
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

	config, err := cfg.ReadDaemonConfig()
	if err != nil {
		return tracerr.Wrap(err)
	}

	if slices.Contains(config.Repos, repoPath) {
		fmt.Println("The Daemon is already monitoring " + repoPath)
	} else {
		config.Repos = append(config.Repos, repoPath)
	}

	err = cfg.WriteDaemonConfig(config)
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

	config, err := cfg.ReadDaemonConfig()
	if err != nil {
		return tracerr.Wrap(err)
	}

	pos := -1
	for i, rp := range config.Repos {
		if rp == repoPath {
			pos = i
			break
		}
	}

	if pos == -1 {
		err = errors.New("repo not tracked")
		return tracerr.Errorf("%w - %s", err, repoPath)
	}

	config.Repos = append(config.Repos[:pos], config.Repos[pos+1:]...)
	err = cfg.WriteDaemonConfig(config)
	if err != nil {
		return tracerr.Wrap(err)
	}

	if len(config.Repos) == 0 {
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

	config, err := cfg.ReadDaemonConfig()
	if err != nil {
		return tracerr.Wrap(err)
	}

	envMap := make(map[string]string, len(config.Envs)+len(vars))
	for _, e := range config.Envs {
		k, v, _ := strings.Cut(e, "=")
		envMap[k] = v
	}
	for _, v := range vars {
		k, val, _ := strings.Cut(v, "=")
		envMap[k] = val
	}

	config.Envs = make([]string, 0, len(envMap))
	for k, v := range envMap {
		config.Envs = append(config.Envs, k+"="+v)
	}
	err = cfg.WriteDaemonConfig(config)
	if err != nil {
		return tracerr.Wrap(err)
	}

	fmt.Println(strings.Join(config.Envs, "\n"))

	return nil
}
