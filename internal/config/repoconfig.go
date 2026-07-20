package config

import (
	"os"
	"time"
)

// RepoConfig holds resolved repository synchronization settings.
type RepoConfig struct {
	RepoPath     string
	SyncInterval time.Duration
	Debounce     time.Duration
	GitExec      string
	Env          []string
}

// @description    Reads and resolves repository synchronization settings.
//
// NewRepoConfig reads global settings from config.json and repository-local [auto-sync] settings,
// merges them with local overriding global overriding defaults, and stats an explicitly configured
// git executable. An unset gitexec defaults to git and is resolved through PATH at subprocess time.
//
// @param           repoPath    "path to the repository root"
//
// @return          RepoConfig  "resolved repository configuration"
//
// @return          error       "nil on success, or an error reading settings or the executable"
func NewRepoConfig(repoPath string) (RepoConfig, error) {
	global, err := ReadGlobalSettings()
	if err != nil {
		return RepoConfig{}, err
	}

	local, err := ReadLocalSettings(repoPath)
	if err != nil {
		return RepoConfig{}, err
	}

	syncInterval, debounce, gitExec := Resolve(global, local)

	// An explicitly configured gitexec (either scope) is stat-checked, matching existing behavior.
	// An unset gitexec defaults to git and is resolved through PATH at subprocess time.
	if (local != nil && local.GitExec != nil) || (global != nil && global.GitExec != nil) {
		if _, err := os.Stat(gitExec); err != nil {
			return RepoConfig{}, err
		}
	}

	return RepoConfig{
		RepoPath:     repoPath,
		SyncInterval: syncInterval,
		Debounce:     debounce,
		GitExec:      gitExec,
	}, nil
}
