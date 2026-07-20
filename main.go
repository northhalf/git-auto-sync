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

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/daemonstate"
	"github.com/northhalf/git-auto-sync/internal/logging"
	"github.com/northhalf/git-auto-sync/internal/notification"
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
			logging.SetupLogger(debug)
			return nil
		},
		Commands: []*cli.Command{
			{
				Name:    "watch",
				Aliases: []string{"monitor", "w", "m"},
				Usage:   "Monitor a folder for changes",
				Before: func(ctx *cli.Context) error {
					notification.WarnIfUnavailable(slog.Default())
					return nil
				},
				Action: func(ctx *cli.Context) error {
					repoPath, err := os.Getwd()
					if err != nil {
						return err
					}

					repoPath, err = isValidGitRepo(repoPath)
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
				Before: func(ctx *cli.Context) error {
					notification.WarnIfUnavailable(slog.Default())
					return nil
				},
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
						return err
					}

					repoPath, err = isValidGitRepo(repoPath)
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
					repoPath, err := os.Getwd()
					if err != nil {
						return err
					}

					repoPath, err = isValidGitRepo(repoPath)
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
			configCommand(),
			{
				Name:    "daemon",
				Aliases: []string{"d"},
				Usage:   "Interact with the background daemon",
				Before: func(ctx *cli.Context) error {
					notification.WarnIfUnavailable(slog.Default())
					return nil
				},
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
	args := sanitizeArgs(os.Args)
	err := app.Run(args)
	if err != nil {
		slog.Error("run failed", "error", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// @description    Drops a Termux-inserted self-path duplicate from argv.
//
// On affected Termux versions (termux/termux-app#4630), the exec wrapper rewrites
// execve(prog, [argv0, args...]) to execve(linker64, [prog, argv0, args...]), so
// Go sees os.Args = [argv0, progAbsolutePath, args...]. urfave/cli then treats
// the inserted progAbsolutePath as an unknown command and reports "No help topic
// for '<path>'". os.Executable() returns linker64 (not prog), so sanitizeArgs
// detects the duplicate by comparing argv[1] against argv[0]: when they name the
// same file, argv[1] is the inserted duplicate and is removed.
//
// @param           argv       "raw command-line arguments, typically os.Args"
//
// @return          []string   "argv with a leading self-path duplicate removed, or argv unchanged"
func sanitizeArgs(argv []string) []string {
	if len(argv) < 2 {
		return argv
	}
	if isSelfDuplicate(argv[0], argv[1]) {
		return append([]string{argv[0]}, argv[2:]...)
	}
	return argv
}

// @description    Reports whether argv[1] is the Termux-inserted executable duplicate.
//
// isSelfDuplicate returns true when argv[1] names the same file as argv[0]
// (absolute or relative path invocation), or for PATH invocation where argv[0]
// is a bare name, when argv[1] is an executable whose basename equals argv[0].
//
// @param           argv0  "user-typed program name, os.Args[0]"
//
// @param           argv1  "suspect duplicate, os.Args[1]"
//
// @return          bool   "true when argv[1] is the inserted executable path"
func isSelfDuplicate(argv0, argv1 string) bool {
	if sameExecutablePath(argv0, argv1) {
		return true
	}
	return filepath.Base(argv1) == argv0 && isExecutableFile(argv1)
}

// @description    Reports whether two paths name the same file.
//
// sameExecutablePath compares the paths directly and, when both stat, by inode
// via os.SameFile so that a relative, symlinked, or Android multi-user aliased
// path still matches. A path that does not exist returns false.
//
// @param           a      "first path"
//
// @param           b      "second path"
//
// @return          bool   "true when both paths resolve to the same file"
func sameExecutablePath(a, b string) bool {
	if a == b {
		return true
	}
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(ia, ib)
}

// @description    Reports whether a path names an executable regular file.
//
// @param           path  "filesystem path to test"
//
// @return          bool  "true when the path is a regular file with any execute bit"
func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}
