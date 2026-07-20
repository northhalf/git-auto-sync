// Package daemonstate persists the daemon's per-repository runtime status so the CLI, a separate
// process, can report whether each monitored repository is running normally or paused awaiting user
// intervention. The daemon writes state.json on watcher state transitions and refreshes a heartbeat
// timestamp periodically; the CLI reads it and treats entries older than StaleThreshold as stale.
package daemonstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// StaleThreshold is the maximum age at which a status entry is considered fresh. It exceeds the
// daemon's configPollInterval so a normally refreshing entry never reads as stale, while a stopped
// daemon is detected within a few intervals.
const StaleThreshold = 3 * time.Minute

// Status is the user-facing runtime status of a monitored repository. Running covers both normal
// operation and remote-failure backoff, which auto-recovers; Paused indicates an error that needs
// user intervention, such as a rebase conflict.
type Status string

const (
	// StatusRunning means the watcher is active and either syncing normally or retrying a remote
	// error.
	StatusRunning Status = "running"
	// StatusPaused means the watcher halted after a non-remote error and needs the daemon restarted
	// or the repository removed and re-added.
	StatusPaused Status = "paused"
)

// RepoStatus is one repository's runtime status as persisted to state.json. Stage records the
// synchronization stage that caused a pause (author, commit, compare, rebase, alert) and is empty
// while running. UpdatedAt is refreshed by the daemon heartbeat so the CLI can detect staleness.
// LastSyncedAt records the time of the most recent successful synchronization; the zero value means
// the repository has never completed a sync. The daemon refreshes it on every successful AutoSync,
// and the CLI sync command refreshes it via RecordSyncSuccess, so the timestamp survives across
// both writer processes through the merge in persistLocked.
type RepoStatus struct {
	Repo         string    `json:"repo"`
	Status       Status    `json:"status"`
	Stage        string    `json:"stage,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
	LastSyncedAt time.Time `json:"lastSyncedAt,omitempty"`
}

// State is the complete contents of state.json: the runtime status of every monitored repository.
type State struct {
	Repos []RepoStatus `json:"repos"`
}

// @description    Resolves the state file path.
//
// StateFile joins the platform configuration directory with the git-auto-sync directory and the
// state.json file name, placing runtime status beside config.json. It does not create the directory
// so callers that only inspect the path, such as the modification-time poller, avoid filesystem side
// effects.
//
// @return          string  "absolute path to the state file"
//
// @return          error   "nil on success, or an error when the platform config directory cannot be resolved"
func StateFile() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "git-auto-sync", "state.json"), nil
}

// @description    Reads the daemon state.
//
// ReadState decodes state.json, returning an empty state when the file does not exist. A corrupt or
// unreadable existing file is a hard error so stale or malformed status is never silently ignored.
//
// @return          *State  "decoded state, or an empty state when no file exists"
//
// @return          error   "nil on success or when the file is absent, or an error resolving, opening, or decoding the file"
func ReadState() (*State, error) {
	stateFile, err := StateFile()
	if err != nil {
		return nil, err
	}

	state := &State{}

	fh, err := os.Open(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, err
	}
	defer func() {
		_ = fh.Close()
	}()

	if err := json.NewDecoder(fh).Decode(state); err != nil {
		return nil, err
	}

	return state, nil
}

// @description    Writes the daemon state atomically.
//
// WriteState encodes state to state.json via a temporary file in the same directory followed by a
// rename, so a CLI process reading the file concurrently with a daemon write never observes a
// partial document. Repositories are sorted by path for stable output.
//
// @param           state  "state to persist"
//
// @return          error  "nil on success, or an error creating the directory, encoding, writing, or renaming the file"
func WriteState(state *State) error {
	stateFile, err := StateFile()
	if err != nil {
		return err
	}

	dir := filepath.Dir(stateFile)
	if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
		return mkErr
	}

	sorted := append([]RepoStatus(nil), state.Repos...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Repo < sorted[j].Repo })

	data, err := json.MarshalIndent(&State{Repos: sorted}, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpName, stateFile); err != nil {
		cleanup()
		return err
	}

	return nil
}

// @description    Records a successful sync for a repository.
//
// RecordSyncSuccess reads state.json, sets repo's LastSyncedAt to the current time, and writes the
// state back atomically. When repo already has an entry, its Status, Stage, and UpdatedAt are
// preserved so a manual CLI sync does not clobber the daemon's runtime status; only LastSyncedAt
// moves forward. When repo has no entry, a running entry is created with UpdatedAt set to the
// current time so a subsequent status read does not treat it as stale. It is intended for the CLI
// sync command, a separate process from the daemon, so it always round-trips through the file and
// never trusts an in-memory copy.
//
// @param           repo   "repository path whose successful sync is being recorded"
//
// @return          error  "nil on success, or an error reading or writing the state file"
func RecordSyncSuccess(repo string) error {
	state, err := ReadState()
	if err != nil {
		return err
	}

	now := time.Now()
	updated := false
	for i := range state.Repos {
		if state.Repos[i].Repo == repo {
			state.Repos[i].LastSyncedAt = now
			updated = true
			break
		}
	}
	if !updated {
		state.Repos = append(state.Repos, RepoStatus{
			Repo:         repo,
			Status:       StatusRunning,
			UpdatedAt:    now,
			LastSyncedAt: now,
		})
	}

	if err := WriteState(state); err != nil {
		return err
	}
	return nil
}

// StateModTime stats state.json and returns its modification time. It returns the zero time with a
// nil error when the file does not exist so callers can treat a missing file as an empty, unchanged
// state.
//
// @return          time.Time  "file modification time, or the zero time when the file is absent"
//
// @return          error      "nil on success or when the file is absent, or an error resolving or stating the path"
func StateModTime() (time.Time, error) {
	stateFile, err := StateFile()
	if err != nil {
		return time.Time{}, err
	}

	info, err := os.Stat(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}

	return info.ModTime(), nil
}

// @description    Reports whether the status entry is stale.
//
// IsStale returns true when the entry was last refreshed more than StaleThreshold ago, or when it was
// never refreshed. The CLI uses it to distinguish a live daemon from one that has stopped updating
// the state file.
//
// @param           now  "reference time, typically the current time"
//
// @return          bool "true when the entry is stale or never refreshed"
func (r RepoStatus) IsStale(now time.Time) bool {
	if r.UpdatedAt.IsZero() {
		return true
	}
	return now.Sub(r.UpdatedAt) > StaleThreshold
}
