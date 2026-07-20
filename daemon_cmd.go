package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kardianos/service"
	cfg "github.com/northhalf/git-auto-sync/internal/config"
	"github.com/northhalf/git-auto-sync/internal/daemonservice"
	"github.com/northhalf/git-auto-sync/internal/daemonstate"
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
		return nil
	case err != nil:
		return tracerr.Wrap(err)
	case status == service.StatusRunning:
		fmt.Println("git-auto-sync-daemon is Running!")
	case status == service.StatusStopped:
		fmt.Println("git-auto-sync-daemon is NOT Running!")
		return nil
	case status == service.StatusUnknown:
		// No user-facing message for an unknown status.
	}

	settings, err := cfg.ReadGlobalSettings()
	if err != nil {
		return tracerr.Wrap(err)
	}

	state, err := daemonstate.ReadState()
	if err != nil {
		return tracerr.Wrap(err)
	}

	fmt.Println("Monitoring - ")
	now := time.Now()
	printRepos(os.Stdout, settings.Repos, state, now, "  ")

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

// @description    Restarts the daemon service.
//
// daemonRestart stops a running daemon service and starts it again, reporting the outcome. When the
// service is not installed it suggests installing with run instead of attempting a restart.
//
// @param           ctx    "CLI context for the daemon restart command"
//
// @return          error  "nil on success or when not installed, or an error querying, stopping, or starting the service"
func daemonRestart(ctx *cli.Context) error {
	s, err := daemonservice.NewServiceWithDaemon(cliDaemon{})
	if err != nil {
		return tracerr.Wrap(err)
	}

	if err := s.Restart(); err != nil {
		if errors.Is(err, daemonservice.ErrNotInstalled) {
			fmt.Println("git-auto-sync-daemon is NOT installed; run `git-auto-sync daemon run` to install")
			return nil
		}
		fmt.Println("git-auto-sync-daemon failed to restart")
		return tracerr.Wrap(err)
	}

	fmt.Println("git-auto-sync-daemon restarted successfully")
	return nil
}

// @description    Uninstalls the daemon service.
//
// daemonUninstall stops and removes the daemon service, reporting the outcome. The monitored
// repository list and environment configuration are preserved so a later `daemon add` or
// `daemon run` picks them up again. When the service is not installed it reports that state without
// attempting an uninstall, matching the stop and restart commands.
//
// @param           ctx    "CLI context for the daemon uninstall command"
//
// @return          error  "nil on success or when not installed, or an error building, querying, or disabling the service"
func daemonUninstall(ctx *cli.Context) error {
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

	if status == service.StatusRunning {
		fmt.Println("git-auto-sync-daemon is running; stopping before uninstall")
	}

	if err := s.Disable(); err != nil {
		fmt.Println("git-auto-sync-daemon failed to uninstall")
		return tracerr.Wrap(err)
	}

	fmt.Println("git-auto-sync-daemon uninstalled successfully")
	return nil
}

// @description    daemonList prints each monitored repository with its runtime status.
//
// daemonList reads the daemon configuration and state file and prints the daemon service status
// followed by one line per repository showing whether it is running normally, paused with a reason,
// or unknown because the daemon is not refreshing its state.
//
// @param           ctx    "CLI context for the daemon list command"
//
// @return          error  "nil on success, or an error reading the configuration or state"
func daemonList(ctx *cli.Context) error {
	settings, err := cfg.ReadGlobalSettings()
	if err != nil {
		return tracerr.Wrap(err)
	}

	state, err := daemonstate.ReadState()
	if err != nil {
		return tracerr.Wrap(err)
	}

	fmt.Println("daemon:", daemonServiceStatus())
	now := time.Now()
	if len(settings.Repos) == 0 {
		fmt.Println("No repository be added")
		return nil
	}
	printRepos(os.Stdout, settings.Repos, state, now, "")
	return nil
}

// @description    Returns a short daemon service status label.
//
// daemonServiceStatus queries the daemon service and returns a lowercase label for display: running,
// not running, not installed, or unknown when the status cannot be determined. It never returns an
// error so list and status output can always show a header line.
//
// @return          string  "short daemon service status label"
func daemonServiceStatus() string {
	s, err := daemonservice.NewServiceWithDaemon(cliDaemon{})
	if err != nil {
		return "unknown"
	}

	status, err := s.Status()
	switch {
	case errors.Is(err, daemonservice.ErrNotInstalled):
		return "not installed"
	case err != nil:
		return "unknown"
	case status == service.StatusRunning:
		return "running"
	case status == service.StatusStopped:
		return "not running"

	default:
		return "unknown"
	}
}

// @description    Returns the human-readable status label of a repository.
//
// repoStatus looks up repoPath in the daemon state and returns "running", "paused (<reason>)", or
// "unknown (daemon may not be running)" when the entry is stale, missing, or the daemon is not
// refreshing state. It does not include the last-sync segment, which is formatted separately so the
// caller can column-align the status and sync fields independently.
//
// @param           repoPath  "repository path to look up"
//
// @param           state     "daemon state read from state.json"
//
// @param           now       "reference time for staleness, typically the current time"
//
// @return          string    "human-readable repository status label"
func repoStatus(repoPath string, state *daemonstate.State, now time.Time) string {
	for _, r := range state.Repos {
		if r.Repo != repoPath {
			continue
		}
		if r.IsStale(now) {
			return "unknown (daemon may not be running)"
		}
		if r.Status == daemonstate.StatusPaused {
			return "paused (" + reasonForStage(r.Stage) + ")"
		}
		return "running"
	}
	return "unknown (daemon may not be running)"
}

// @description    Returns the last-sync segment for a repository.
//
// repoLastSynced looks up repoPath in the daemon state and returns the formatted last-sync segment,
// or an empty string when the entry is stale or missing, since a stale or absent entry gives no
// trustworthy sync time. A fresh entry with a zero LastSyncedAt returns "never synced".
//
// @param           repoPath  "repository path to look up"
//
// @param           state     "daemon state read from state.json"
//
// @param           now       "reference time for the relative component, typically the current time"
//
// @return          string    "formatted last-sync segment, or empty when the entry is stale or missing"
func repoLastSynced(repoPath string, state *daemonstate.State, now time.Time) string {
	for _, r := range state.Repos {
		if r.Repo != repoPath {
			continue
		}
		if r.IsStale(now) {
			return ""
		}
		return formatLastSynced(r.LastSyncedAt, now)
	}
	return ""
}

// @description    Formats a last-sync time for display.
//
// formatLastSynced returns "never synced" when last is the zero time, otherwise
// "synced <absolute> (<relative> ago)" using local time. The absolute timestamp is fixed-width; the
// relative suffix varies, so the right edge is intentionally ragged rather than padded.
//
// @param           last  "last successful sync time, or the zero time when never synced"
//
// @param           now   "reference time for the relative component, typically the current time"
//
// @return          string "formatted last-sync segment"
func formatLastSynced(last, now time.Time) string {
	if last.IsZero() {
		return "never synced"
	}
	abs := last.Local().Format("2006-01-02 15:04:05")
	return "synced " + abs + " (" + humanDuration(now.Sub(last)) + " ago)"
}

// @description    Renders a duration as a compound human-readable string.
//
// humanDuration formats d as a compound "<D>d<H>h<M>m<S>s" string, omitting leading zero parts so
// only present units appear. Seconds are dropped once a minute or coarser unit is present, so 5m30s
// reads as "5m". Durations of 7 days or more collapse to just "<D>d", dropping the hour, minute, and
// second components. Durations under a minute read as "<S>s"; the zero duration reads as "0s".
// Negative durations, which arise only from clock skew on a future LastSyncedAt, are clamped to "0s".
//
// @param           d     "elapsed time since the last sync"
//
// @return          string "compound human-readable duration label"
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d >= 7*24*time.Hour {
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	}

	days := int(d / (24 * time.Hour))
	rem := d % (24 * time.Hour)
	hours := int(rem / time.Hour)
	rem = rem % time.Hour
	minutes := int(rem / time.Minute)
	rem = rem % time.Minute
	seconds := int(rem / time.Second)

	var b strings.Builder
	if days > 0 {
		b.WriteString(strconv.Itoa(days))
		b.WriteByte('d')
	}
	if hours > 0 {
		b.WriteString(strconv.Itoa(hours))
		b.WriteByte('h')
	}
	if minutes > 0 {
		b.WriteString(strconv.Itoa(minutes))
		b.WriteByte('m')
	}
	// Seconds appear only when no coarser unit is present (duration under a minute).
	if days == 0 && hours == 0 && minutes == 0 {
		b.WriteString(strconv.Itoa(seconds))
		b.WriteByte('s')
	}
	if b.Len() == 0 {
		return "0s"
	}
	return b.String()
}

// @description    Pads a string with trailing spaces to a rune width.
//
// padRight returns s followed by enough spaces that its rune count reaches width. Strings already at
// or beyond width are returned unchanged. Width is measured in runes so multi-byte paths align on the
// rune grid; it does not account for East-Asian full-width glyphs, which is acceptable for filesystem
// paths and keeps the implementation stdlib-only.
//
// @param           s      "string to pad"
//
// @param           width  "target rune count"
//
// @return          string "s padded to width runes, or s unchanged when already at or beyond width"
func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// @description    Prints monitored repositories with aligned columns.
//
// printRepos writes one line per repository path to w, aligning the path and status columns to the
// widest entry so the separators line up. Lines whose status is "unknown (daemon may not be running)"
// omit the last-sync segment, since a stale or absent entry carries no trustworthy sync time. indent
// is prepended to every line so the status command can nest rows under its header while the list
// command prints flush-left.
//
// @param           w       "writer that receives the formatted repository lines"
//
// @param           repos   "repository paths to print, in display order"
//
// @param           state   "daemon state read from state.json"
//
// @param           now     "reference time for staleness and relative durations"
//
// @param           indent  "prefix prepended to each repository line"
func printRepos(w io.Writer, repos []string, state *daemonstate.State, now time.Time, indent string) {
	maxPathW, maxStatusW := 0, 0
	statuses := make([]string, len(repos))
	synced := make([]string, len(repos))
	for i, repoPath := range repos {
		statuses[i] = repoStatus(repoPath, state, now)
		synced[i] = repoLastSynced(repoPath, state, now)
		if w := utf8.RuneCountInString(repoPath); w > maxPathW {
			maxPathW = w
		}
		if w := utf8.RuneCountInString(statuses[i]); w > maxStatusW {
			maxStatusW = w
		}
	}

	for i, repoPath := range repos {
		line := indent + padRight(repoPath, maxPathW) + "  -  " + padRight(statuses[i], maxStatusW)
		if synced[i] != "" {
			line += "  -  " + synced[i]
		}
		_, _ = fmt.Fprintln(w, line)
	}
}

// @description    Maps a paused synchronization stage to a reason.
//
// reasonForStage returns the user-readable reason for a pause, defaulting to "unknown error" for an
// unrecognized stage so the output always carries a meaningful cause.
//
// @param           stage  "synchronization stage that caused the pause"
//
// @return          string  "human-readable reason"
func reasonForStage(stage string) string {
	switch stage {
	case "rebase":
		return "rebase conflict"
	case "author":
		return "git author not configured"
	case "commit":
		return "commit failed"
	case "compare":
		return "upstream comparison failed"
	case "repo-busy":
		return "git operation in progress (rebase/merge/...)"
	case "detached-head":
		return "HEAD is detached"
	case "no-upstream":
		return "branch has no upstream"
	default:
		return "unknown error"
	}
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
