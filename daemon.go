package main

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/northhalf/git-auto-sync/common"
	cfg "github.com/northhalf/git-auto-sync/common/config"
	"github.com/urfave/cli/v2"
	"github.com/ztrue/tracerr"
	"gopkg.in/src-d/go-git.v4"
)

var errRepoPathInvalid = errors.New("not a valid git repo")

// @description    Prints daemon status and repositories.
//
// daemonStatus prints the service status and every repository in the daemon configuration,
// returning an error if either cannot be read.
//
// @param           ctx    "CLI context for the daemon status command"
//
// @return          error  "nil on success, or an error from the service or configuration"
func daemonStatus(ctx *cli.Context) error {
	s, err := common.NewService()
	if err != nil {
		return tracerr.Wrap(err)
	}

	err = s.Status()
	if err != nil {
		return tracerr.Wrap(err)
	}

	config, err := cfg.Read()
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
	config, err := cfg.Read()
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
// configuration, and enables the user service. Enabling may stop a running service before
// installing or reinstalling and starting it.
//
// @param           ctx    "CLI context containing the repository path"
//
// @return          error  "nil on success, or an error validating, persisting, or enabling the service"
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

	config, err := cfg.Read()
	if err != nil {
		return tracerr.Wrap(err)
	}

	if slices.Contains(config.Repos, repoPath) {
		fmt.Println("The Daemon is already monitoring " + repoPath)
	} else {
		config.Repos = append(config.Repos, repoPath)
	}

	err = cfg.Write(config)
	if err != nil {
		return tracerr.Wrap(err)
	}

	s, err := common.NewService()
	if err != nil {
		return tracerr.Wrap(err)
	}

	err = s.Enable()
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}

// @description    Validates a Git worktree path.
//
// isValidGitRepo verifies that a caller-provided path belongs to a non-bare Git worktree and walks
// upward to find the repository root containing a .git directory.
//
// @param           repoPath  "caller-provided path to validate as a Git repository or descendant"
//
// @return          string    "repository root derived from the caller-provided path"
//
// @return          error     "nil on success, or an error for an invalid path or repository"
func isValidGitRepo(repoPath string) (string, error) {
	info, err := os.Stat(repoPath)
	if os.IsNotExist(err) {
		return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
	}

	if !info.IsDir() {
		return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
	}

	_, err = git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", tracerr.Errorf("Not a valid git repo - %s\n%w", repoPath, err)
	}

	for {
		info, err := os.Stat(filepath.Join(repoPath, ".git"))
		if err != nil {
			if !os.IsNotExist(err) {
				return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
			}
		}

		if os.IsNotExist(err) {
			repoPath = filepath.Dir(repoPath)
			continue
		}

		if !info.IsDir() {
			return "", tracerr.Errorf("%w - %s", errRepoPathInvalid, repoPath)
		}
		break
	}

	return repoPath, nil
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

	config, err := cfg.Read()
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

	config.Repos = remove(config.Repos, pos)
	err = cfg.Write(config)
	if err != nil {
		return tracerr.Wrap(err)
	}

	if len(config.Repos) == 0 {
		s, err := common.NewService()
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

	config, err := cfg.Read()
	if err != nil {
		return tracerr.Wrap(err)
	}

	envMap := toEnvMap(config.Envs)
	newMap := toEnvMap(vars)

	maps.Copy(envMap, newMap)

	config.Envs = toEnvStrings(envMap)
	err = cfg.Write(config)
	if err != nil {
		return tracerr.Wrap(err)
	}

	fmt.Println(strings.Join(config.Envs, "\n"))

	return nil
}

// @description    remove deletes the element at an index by joining the surrounding slices.
//
// @param           slice     "string slice to modify"
//
// @param           s         "index of the element to remove"
//
// @return          []string  "slice without the indexed element"
func remove(slice []string, s int) []string {
	return append(slice[:s], slice[s+1:]...)
}

// @description    toEnvMap parses key=value entries into a map, preserving equals signs in values.
//
// @param           envs               "environment entries to parse"
//
// @return          map[string]string  "environment values keyed by variable name"
func toEnvMap(envs []string) map[string]string {
	m := map[string]string{}
	for _, e := range envs {
		parts := strings.Split(e, "=")
		if len(parts) > 1 {
			m[parts[0]] = strings.Join(parts[1:], "=")
		} else {
			m[e] = ""
		}
	}

	return m
}

// @description    toEnvStrings formats an environment map as key=value entries.
//
// @param           m         "environment values keyed by variable name"
//
// @return          []string  "formatted environment entries"
func toEnvStrings(m map[string]string) []string {
	vals := []string{}
	for k, v := range m {
		x := fmt.Sprintf("%s=%s", k, v)
		vals = append(vals, x)
	}

	return vals
}
