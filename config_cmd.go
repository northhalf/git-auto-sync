package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	cfg "github.com/northhalf/git-auto-sync/internal/config"
	"github.com/urfave/cli/v2"
)

// errConfigUsage signals a config command usage error.
var errConfigUsage = errors.New("usage: git-auto-sync config [--global|--local] [--get|--list|--unset] <key> [value]")

// @description    Dispatches the config command.
//
// configCmd resolves the scope from --global/--local, then dispatches --list, --unset, a two-arg
// write, or a one-arg read. The default scope is global for writes/unset and the merged effective
// value for reads/list.
//
// @param           ctx    "CLI context for the config command"
//
// @return          error  "nil on success, or a usage, validation, or storage error"
func configCmd(ctx *cli.Context) error {
	if ctx.Bool("global") && ctx.Bool("local") {
		return errConfigUsage
	}

	args := ctx.Args().Slice()

	if ctx.Bool("list") {
		return configList(ctx)
	}
	if ctx.Bool("unset") {
		if len(args) != 1 {
			return errConfigUsage
		}
		return configUnset(ctx, args[0])
	}
	if len(args) == 2 && !ctx.Bool("get") {
		return configSet(ctx, args[0], args[1])
	}
	if len(args) == 1 {
		return configGet(ctx, args[0])
	}
	return errConfigUsage
}

// @description    Reports whether the command targets the repository-local config.
//
// @param           ctx    "CLI context carrying the scope flags"
//
// @return          bool   "true when --local is set"
func scopeLocal(ctx *cli.Context) bool { return ctx.Bool("local") }

// @description    Writes a setting at the resolved scope.
//
// configSet validates key and value, then writes to global config.json (default and --global) or
// the repository .git/config (--local).
//
// @param           ctx     "CLI context carrying the scope flags"
//
// @param           key     "one of syncInterval, debounce, gitexec"
//
// @param           value   "validated value to store"
//
// @return          error   "nil on success, or a validation or storage error"
func configSet(ctx *cli.Context, key, value string) error {
	field, ok := cfg.SettingKeys[key]
	if !ok {
		return fmt.Errorf("unknown setting: %s", key)
	}
	parsed, err := parseValue(key, value)
	if err != nil {
		return err
	}

	if scopeLocal(ctx) {
		repoPath, err := currentRepoPath()
		if err != nil {
			return err
		}
		return cfg.SetLocalSetting(repoPath, key, parsed)
	}

	settings, err := cfg.ReadGlobalSettings()
	if err != nil {
		return err
	}
	if err := field.Decode(settings, parsed); err != nil {
		return err
	}
	return cfg.WriteGlobalSettings(settings)
}

// @description    Reads a setting at the resolved scope.
//
// configGet prints the effective value (default and --get, merged local over global over default)
// or only the local raw value (--local). Local reads print nothing when the key is unset.
//
// @param           ctx     "CLI context carrying the scope flags"
//
// @param           key     "one of syncInterval, debounce, gitexec"
//
// @return          error   "nil on success, or a storage error"
func configGet(ctx *cli.Context, key string) error {
	field, ok := cfg.SettingKeys[key]
	if !ok {
		return fmt.Errorf("unknown setting: %s", key)
	}

	if scopeLocal(ctx) {
		repoPath, err := currentRepoPath()
		if err != nil {
			return err
		}
		local, err := cfg.ReadLocalSettings(repoPath)
		if err != nil {
			return err
		}
		if v, ok := field.Raw(local); ok {
			_, _ = fmt.Fprintln(ctx.App.Writer, v)
		}
		return nil
	}

	sync, debounce, gitExec, err := effectiveSettings()
	if err != nil {
		return err
	}
	switch key {
	case "syncInterval":
		_, _ = fmt.Fprintln(ctx.App.Writer, int(sync/time.Minute))
	case "debounce":
		_, _ = fmt.Fprintln(ctx.App.Writer, int(debounce/time.Minute))
	case "gitexec":
		_, _ = fmt.Fprintln(ctx.App.Writer, gitExec)
	}
	return nil
}

// @description    Lists all three effective settings.
//
// configList prints syncInterval, debounce, and gitexec effective values.
//
// @param           ctx     "CLI context carrying the scope flags"
//
// @return          error   "nil on success, or a storage error"
func configList(ctx *cli.Context) error {
	sync, debounce, gitExec, err := effectiveSettings()
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(ctx.App.Writer, "syncInterval="+strconv.Itoa(int(sync/time.Minute)))
	_, _ = fmt.Fprintln(ctx.App.Writer, "debounce="+strconv.Itoa(int(debounce/time.Minute)))
	_, _ = fmt.Fprintln(ctx.App.Writer, "gitexec="+gitExec)
	return nil
}

// @description    Resolves the effective synchronization settings.
//
// effectiveSettings reads the global settings, merges repository-local settings when the working
// directory is inside a repository, and resolves the effective values.
//
// @return          time.Duration  "effective sync interval"
//
// @return          time.Duration  "effective debounce"
//
// @return          string         "effective git executable"
//
// @return          error          "nil on success, or a storage error"
func effectiveSettings() (time.Duration, time.Duration, string, error) {
	global, err := cfg.ReadGlobalSettings()
	if err != nil {
		return 0, 0, "", err
	}
	local := &cfg.Settings{}
	repoPath, repoErr := currentRepoPath()
	if repoErr == nil {
		local, err = cfg.ReadLocalSettings(repoPath)
		if err != nil {
			return 0, 0, "", err
		}
	}
	sync, debounce, gitExec := cfg.Resolve(global, local)
	return sync, debounce, gitExec, nil
}

// @description    Removes a setting at the resolved scope.
//
// configUnset removes key from global config.json (default and --global) or the repository
// .git/config (--local).
//
// @param           ctx     "CLI context carrying the scope flags"
//
// @param           key     "one of syncInterval, debounce, gitexec"
//
// @return          error   "nil on success, or a storage error"
func configUnset(ctx *cli.Context, key string) error {
	field, ok := cfg.SettingKeys[key]
	if !ok {
		return fmt.Errorf("unknown setting: %s", key)
	}

	if scopeLocal(ctx) {
		repoPath, err := currentRepoPath()
		if err != nil {
			return err
		}
		return cfg.UnsetLocalSetting(repoPath, key)
	}

	settings, err := cfg.ReadGlobalSettings()
	if err != nil {
		return err
	}
	field.Clear(settings)
	return cfg.WriteGlobalSettings(settings)
}

// @description    Validates and normalizes a value for key before storage.
//
// parseValue parses syncInterval and debounce as positive integers and stats the gitexec path,
// returning the normalized storage string.
//
// @param           key      "one of syncInterval, debounce, gitexec"
//
// @param           value    "raw value to validate"
//
// @return          string   "normalized value to store"
//
// @return          error    "nil on success, or a validation error"
func parseValue(key, value string) (string, error) {
	switch key {
	case "syncInterval", "debounce":
		n, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("invalid %s: %s", key, value)
		}
		if n <= 0 {
			return "", fmt.Errorf("%s must be positive: %d", key, n)
		}
		return strconv.Itoa(n), nil
	case "gitexec":
		if _, err := os.Stat(value); err != nil {
			return "", err
		}
		return value, nil
	}
	return "", fmt.Errorf("unknown setting: %s", key)
}

// @description    Returns the repository root for the current directory.
//
// currentRepoPath gets the working directory and validates it as a Git repository.
//
// @return          string   "absolute path to the repository root"
//
// @return          error    "nil on success, or an error from getwd or repository validation"
func currentRepoPath() (string, error) {
	repoPath, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return isValidGitRepo(repoPath)
}
