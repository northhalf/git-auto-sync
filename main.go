package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/daemonstate"
	"github.com/northhalf/git-auto-sync/internal/logging"
	"github.com/northhalf/git-auto-sync/internal/notification"
	"github.com/northhalf/git-auto-sync/internal/syncer"
	"github.com/northhalf/git-auto-sync/internal/termux"
	"github.com/northhalf/git-auto-sync/internal/watcher"
)

//go:embed .version
var version string

// @description    Warns when desktop notifications are unavailable.
//
// warnIfNotificationUnavailable is the shared Before hook for commands that may trigger
// desktop notifications.
//
// @param           _      "CLI context, unused"
//
// @return          error  "always nil"
func warnIfNotificationUnavailable(_ *cli.Context) error {
	notification.WarnIfUnavailable(slog.Default())
	return nil
}

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
			logging.SetupLogger(debug)
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:    "watch",
				Aliases: []string{"monitor", "w", "m"},
				Usage:   "Monitor a folder for changes",
				Before:  warnIfNotificationUnavailable,
				Action: func(ctx *cli.Context) error {
					repoPath, err := currentRepoPath()
					if err != nil {
						return err
					}

					cfg, err := config.NewRepoConfig(repoPath)
					if err != nil {
						return err
					}

					return watcher.WatchForChanges(context.Background(), logging.WithRepo(repoPath), cfg, nil)
				},
			},
			{
				Name:    "sync",
				Aliases: []string{"s"},
				Usage:   "Sync a repo right now",
				Before:  warnIfNotificationUnavailable,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:    "env",
						Aliases: []string{"e"},
						Usage:   "Env variables to pass",
					},
				},
				Action: func(ctx *cli.Context) error {
					repoPath, err := currentRepoPath()
					if err != nil {
						return err
					}

					cfg, err := config.NewRepoConfig(repoPath)
					if err != nil {
						return err
					}

					cfg.Env = append(cfg.Env, ctx.StringSlice("env")...)

					err = syncer.AutoSync(logging.WithRepo(repoPath), cfg)
					if err != nil {
						return err
					}

					if recordErr := daemonstate.RecordSyncSuccess(repoPath); recordErr != nil {
						slog.Warn("record sync success failed", "error", recordErr)
					}

					return nil
				},
			},
			{
				Name:  "check",
				Usage: "Check if a file will be ignored",
				Action: func(ctx *cli.Context) error {
					repoPath, err := currentRepoPath()
					if err != nil {
						return err
					}

					path := ctx.Args().First()
					if strings.TrimSpace(path) == "" {
						return errors.New("missing file path argument")
					}
					path, err = filepath.Abs(path)
					if err != nil {
						return err
					}

					ignored, err := syncer.ShouldIgnoreFile(repoPath, path)
					if err != nil {
						return err
					}
					fmt.Println("Ignored:", ignored)

					return nil
				},
			},
			{
				Name:  "config",
				Usage: "Get, set, or unset git-auto-sync settings",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "global", Usage: "Operate on the global config.json"},
					&cli.BoolFlag{Name: "local", Usage: "Operate on the repository's .git/config"},
					&cli.BoolFlag{Name: "get", Usage: "Print the effective value of a key"},
					&cli.BoolFlag{Name: "list", Aliases: []string{"l"}, Usage: "List all settings"},
					&cli.BoolFlag{Name: "unset", Aliases: []string{"u"}, Usage: "Remove a key"},
				},
				Action: configCmd,
			},
			{
				Name:    "daemon",
				Aliases: []string{"d"},
				Usage:   "Interact with the background daemon",
				Before:  warnIfNotificationUnavailable,
				Subcommands: []*cli.Command{
					{
						Name:   "status",
						Usage:  "Show the Daemon's status",
						Action: daemonStatus,
					},
					{
						Name:   "run",
						Usage:  "Start the daemon service",
						Action: daemonRun,
					},
					{
						Name:   "stop",
						Usage:  "Stop the daemon service",
						Action: daemonStop,
					},
					{
						Name:   "restart",
						Usage:  "Restart the daemon service",
						Action: daemonRestart,
					},
					{
						Name:   "uninstall",
						Usage:  "Uninstall the daemon service",
						Action: daemonUninstall,
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

	// Sanitize argv before dispatching on Android: affected Termux versions insert the
	// executable's own path as argv[1] (termux/termux-app#4630), which would
	// otherwise make urfave/cli report "No help topic for '<path>'" and exit.
	args := os.Args
	if runtime.GOOS == "android" {
		args = termux.SanitizeArgs(args)
	}
	err := app.Run(args)
	if err != nil {
		slog.Error("run failed", "error", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
