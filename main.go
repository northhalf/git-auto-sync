package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
	"github.com/ztrue/tracerr"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/daemonstate"
	"github.com/northhalf/git-auto-sync/internal/logging"
	"github.com/northhalf/git-auto-sync/internal/syncer"
	"github.com/northhalf/git-auto-sync/internal/watcher"
)

//go:embed .version
var version string

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
				Before:  notificationAvailabilityHook,
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

					return watcher.WatchForChanges(context.Background(), logging.WithRepo(repoPath), cfg, nil)
				},
			},
			{
				Name:    "sync",
				Aliases: []string{"s"},
				Usage:   "Sync a repo right now",
				Before:  notificationAvailabilityHook,
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
			configCommand(),
			{
				Name:    "daemon",
				Aliases: []string{"d"},
				Usage:   "Interact with the background daemon",
				Before:  notificationAvailabilityHook,
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

	// Sanitize argv before dispatching: affected Termux versions insert the
	// executable's own path as argv[1] (termux/termux-app#4630), which would
	// otherwise make urfave/cli report "No help topic for '<path>'" and exit.
	exe, _ := os.Executable()
	err := app.Run(sanitizeArgs(os.Args, exe))
	if err != nil {
		slog.Error("run failed", "error", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// @description    Drops a Termux-inserted self-path duplicate from argv.
//
// On affected Termux versions (termux/termux-app#4630), launching a CGO_ENABLED=0 Go
// binary inserts the executable's own path as argv[1], shifting the user's real
// arguments one position later so urfave/cli reports "No help topic for '<path>'".
// sanitizeArgs removes that leading duplicate when argv[1] resolves to the running
// executable, leaving argv unchanged otherwise.
//
// @param           argv       "raw command-line arguments, typically os.Args"
//
// @param           exe        "running executable path from os.Executable, empty when unavailable"
//
// @return          []string   "argv with a leading self-path duplicate removed, or argv unchanged"
func sanitizeArgs(argv []string, exe string) []string {
	if len(argv) < 2 || exe == "" {
		return argv
	}
	if sameExecutablePath(argv[1], exe) {
		return append(argv[:1], argv[2:]...)
	}
	return argv
}

// @description    Reports whether two paths name the same executable file.
//
// sameExecutablePath compares the paths directly and, when both resolve, after
// evaluating symbolic links so that a relative or symlinked argv entry still matches
// the executable's real path. A path that does not exist on disk returns false.
//
// @param           a      "first path, typically an argv entry"
//
// @param           b      "second path, typically the os.Executable result"
//
// @return          bool   "true when both paths resolve to the same file"
func sameExecutablePath(a, b string) bool {
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return false
	}
	return ra == rb
}
