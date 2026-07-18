package daemonstate

import (
	"log/slog"
	"sync"
	"time"
)

// Recorder is the daemon-side writer for state.json. It holds the per-repository status in memory,
// deduplicates unchanged writes, and serializes persistence so concurrent watcher goroutines can
// report transitions safely. The CLI reads the file directly; it never touches the Recorder.
type Recorder struct {
	mu     sync.Mutex
	states map[string]RepoStatus
}

// @description    Creates a daemon state recorder.
//
// NewRecorder returns a Recorder with an empty in-memory state. The first Set for any repository
// creates state.json; Remove of the last repository leaves an empty state file behind.
//
// @return          *Recorder  "ready-to-use recorder with no repositories"
func NewRecorder() *Recorder {
	return &Recorder{states: make(map[string]RepoStatus)}
}

// @description    Sets a repository's status, persisting only on change.
//
// Set updates the in-memory status for repo and writes state.json when the status, stage, or last
// sync time differs from the current entry. Repeated identical reports are no-ops, so frequent
// running reports do not rewrite the file; the heartbeat refreshes UpdatedAt independently. lastSynced
// is the time of the most recent successful sync, or the zero time when the repository has not yet
// completed a sync (such as the watcher's initial report). Write errors are logged and swallowed so
// a transient state-file failure never stops the watcher.
//
// @param           repo         "repository path the status applies to"
//
// @param           status       "running or paused"
//
// @param           stage        "synchronization stage that caused a pause, or empty when running"
//
// @param           lastSynced   "time of the most recent successful sync, or the zero time when none yet"
func (r *Recorder) Set(repo string, status Status, stage string, lastSynced time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.states[repo]
	if ok && existing.Status == status && existing.Stage == stage && existing.LastSyncedAt.Equal(lastSynced) {
		return
	}

	r.states[repo] = RepoStatus{
		Repo:         repo,
		Status:       status,
		Stage:        stage,
		LastSyncedAt: lastSynced,
		UpdatedAt:    time.Now(),
	}
	r.persistLocked()
}

// @description    Refreshes the heartbeat timestamp of every repository.
//
// Heartbeat bumps UpdatedAt to the current time for all tracked repositories and persists state.json
// so the CLI can confirm the daemon is alive. It is a no-op when no repositories are tracked. Write
// errors are logged and swallowed.
func (r *Recorder) Heartbeat() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.states) == 0 {
		return
	}

	now := time.Now()
	for repo, s := range r.states {
		s.UpdatedAt = now
		r.states[repo] = s
	}
	r.persistLocked()
}

// @description    Removes a repository's status entry.
//
// Remove deletes the entry for repo and persists state.json. It is a no-op when repo is not tracked,
// so calling it for an already-removed repository is safe. Write errors are logged and swallowed.
//
// @param           repo  "repository path whose status entry should be removed"
func (r *Recorder) Remove(repo string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.states[repo]; !ok {
		return
	}

	delete(r.states, repo)
	r.persistLocked()
}

// persistLocked encodes the in-memory state to state.json. The caller must hold r.mu. Write errors
// are logged and not returned so transient failures do not propagate into the watcher loop.
//
// Before encoding, persistLocked re-reads state.json and adopts any on-disk LastSyncedAt that is
// newer than the in-memory value (or any non-zero on-disk value when the in-memory value is zero).
// The CLI sync command writes LastSyncedAt from a separate process between daemon writes; without
// this merge, the daemon's next full-file write would clobber that timestamp. The daemon remains the
// sole writer from its own process, so the read-modify-write here is race-free against other daemon
// code paths; a concurrent CLI write either lands before the read (and is preserved) or after the
// rename (and survives until the next persist).
func (r *Recorder) persistLocked() {
	if disk, err := ReadState(); err == nil {
		for i := range r.states {
			mem := r.states[i]
			for _, d := range disk.Repos {
				if d.Repo != mem.Repo {
					continue
				}
				// After treats the zero time as earlier than any real time, so a zero on either
				// side is handled correctly: a real disk value beats a zero memory value (daemon
				// restart, or a CLI sync that wrote disk between daemon writes), and a real memory
				// value is kept when the disk has none or is older.
				if d.LastSyncedAt.After(mem.LastSyncedAt) {
					mem.LastSyncedAt = d.LastSyncedAt
				}
				break
			}
			r.states[i] = mem
		}
	} else {
		slog.Warn("read daemon state for merge failed", "error", err)
	}

	out := &State{Repos: make([]RepoStatus, 0, len(r.states))}
	for _, s := range r.states {
		out.Repos = append(out.Repos, s)
	}
	if err := WriteState(out); err != nil {
		slog.Error("write daemon state failed", "error", err)
	}
}
