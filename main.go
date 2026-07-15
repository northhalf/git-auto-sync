package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/ztrue/tracerr"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/logging"
	"github.com/northhalf/git-auto-sync/internal/syncer"
	"github.com/northhalf/git-auto-sync/internal/watcher"
)

var version = "dev"

// @description    Runs the command-line application.
//
// main builds the command-line application, configures logging, runs the requested command, and
// terminates with a nonzero status when command execution fails.
func main() {
	app := &cli.App{
		Name:                 "git-auto-sync",
		Version:              version,
		Usage:                "Automatically Sync any Git Repo",
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "debug",
				Usage:   "Enable debug logging to stdout",
				EnvVars: []string{"DEBUG"},
			},
		},
		Before: func(ctx *cli.Context) error {
			debug := ctx.Bool("debug") || os.Getenv("DEBUG") == "true"
			_, _ = logging.SetupLogger(debug)
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:    "watch",
				Aliases: []string{"monitor", "w", "m"},
				Usage:   "Monitor a folder for changes",
				Action: func(ctx *cli.Context) error {
					repoPath, err := os.Getwd()
					if err != nil {
						return tracerr.Wrap(err)
					}

					repoPath, err = isValidGitRepo(repoPath)
					if err != nil {
						return tracerr.Wrap(err)
					}

					cfg, err := config.NewRepoConfig(repoPath)
					if err != nil {
						return tracerr.Wrap(err)
					}

					return watcher.WatchForChanges(logging.WithRepo(repoPath), cfg)
				},
			},
			{
				Name:    "sync",
				Aliases: []string{"s"},
				Usage:   "Sync a repo right now",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:    "env",
						Aliases: []string{"e"},
						Usage:   "Env variables to pass",
					},
				},
				Action: func(ctx *cli.Context) error {
					repoPath, err := os.Getwd()
					if err != nil {
						return tracerr.Wrap(err)
					}

					repoPath, err = isValidGitRepo(repoPath)
					if err != nil {
						return tracerr.Wrap(err)
					}

					cfg, err := config.NewRepoConfig(repoPath)
					if err != nil {
						return tracerr.Wrap(err)
					}

					cfg.Env = append(cfg.Env, ctx.StringSlice("env")...)

					err = syncer.AutoSync(logging.WithRepo(repoPath), cfg)
					if err != nil {
						return tracerr.Wrap(err)
					}

					return nil
				},
			},
			{
				Name:  "check",
				Usage: "Check if a file will be ignored",
				Action: func(ctx *cli.Context) error {
					repoPath, err := os.Getwd()
					if err != nil {
						return tracerr.Wrap(err)
					}

					repoPath, err = isValidGitRepo(repoPath)
					if err != nil {
						return tracerr.Wrap(err)
					}

					path := ctx.Args().First()
					if strings.TrimSpace(path) == "" {
						return errors.New("missing file path argument")
					}
					path, err = filepath.Abs(path)
					if err != nil {
						return tracerr.Wrap(err)
					}

					ignored, err := syncer.ShouldIgnoreFile(repoPath, path)
					if err != nil {
						return tracerr.Wrap(err)
					}
					fmt.Println("Ignored:", ignored)

					return nil
				},
			},
			{
				Name:    "daemon",
				Aliases: []string{"d"},
				Usage:   "Interact with the background daemon",
				Subcommands: []*cli.Command{
					{
						Name:   "status",
						Usage:  "Show the Daemon's status",
						Action: daemonStatus,
					},
					{
						Name:    "list",
						Aliases: []string{"ls"},
						Usage:   "List of repos being auto-synced",
						Action:  daemonList,
					},
					{
						Name:   "add",
						Usage:  "Add a repo for auto-sync",
						Action: daemonAdd,
					},
					{
						Name:    "remove",
						Aliases: []string{"rm"},
						Usage:   "Remove a repo from auto-sync",
						Action:  daemonRm,
					},
					{
						Name:   "env",
						Usage:  "Set an environment variable",
						Action: daemonEnv,
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		slog.Error("run failed", "error", err)
		os.Exit(1)
	}
}
