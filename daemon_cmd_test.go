package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/northhalf/git-auto-sync/internal/daemonstate"
	"gotest.tools/v3/assert"
)

// @description    Verifies the repository status label formatting.
//
// Test_RepoStatus covers the running, paused-with-reason, stale, and missing-entry cases so the ls
// and status output carries the expected status label for each watcher state, independent of the
// last-sync segment.
//
// @param           t  "test handle used for table-driven assertions"
func Test_RepoStatus(t *testing.T) {
	now := time.Unix(10_000, 0)
	fresh := now.Add(-30 * time.Second)
	stale := now.Add(-(daemonstate.StaleThreshold + time.Minute))

	tests := []struct {
		name  string
		repo  string
		state *daemonstate.State
		want  string
	}{
		{
			name: "running fresh",
			repo: "/repo",
			state: &daemonstate.State{Repos: []daemonstate.RepoStatus{
				{Repo: "/repo", Status: daemonstate.StatusRunning, UpdatedAt: fresh},
			}},
			want: "running",
		},
		{
			name: "paused rebase",
			repo: "/repo",
			state: &daemonstate.State{Repos: []daemonstate.RepoStatus{
				{Repo: "/repo", Status: daemonstate.StatusPaused, Stage: "rebase", UpdatedAt: fresh},
			}},
			want: "paused (rebase conflict)",
		},
		{
			name: "paused author",
			repo: "/repo",
			state: &daemonstate.State{Repos: []daemonstate.RepoStatus{
				{Repo: "/repo", Status: daemonstate.StatusPaused, Stage: "author", UpdatedAt: fresh},
			}},
			want: "paused (git author not configured)",
		},
		{
			name: "paused unknown stage",
			repo: "/repo",
			state: &daemonstate.State{Repos: []daemonstate.RepoStatus{
				{Repo: "/repo", Status: daemonstate.StatusPaused, Stage: "mystery", UpdatedAt: fresh},
			}},
			want: "paused (unknown error)",
		},
		{
			name: "stale entry",
			repo: "/repo",
			state: &daemonstate.State{Repos: []daemonstate.RepoStatus{
				{Repo: "/repo", Status: daemonstate.StatusRunning, UpdatedAt: stale},
			}},
			want: "unknown (daemon may not be running)",
		},
		{
			name:  "missing entry",
			repo:  "/repo",
			state: &daemonstate.State{},
			want:  "unknown (daemon may not be running)",
		},
		{
			name: "unrelated repo in state",
			repo: "/repo",
			state: &daemonstate.State{Repos: []daemonstate.RepoStatus{
				{Repo: "/other", Status: daemonstate.StatusRunning, UpdatedAt: fresh},
			}},
			want: "unknown (daemon may not be running)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := repoStatus(tc.repo, tc.state, now)
			assert.Equal(t, got, tc.want)
		})
	}
}

// @description    Verifies the last-sync segment for each entry state.
//
// Test_RepoLastSynced covers the synced, never-synced, stale, and missing cases so the ls and status
// output carries the expected last-sync segment (or none) for each watcher state.
//
// @param           t  "test handle used for table-driven assertions"
func Test_RepoLastSynced(t *testing.T) {
	// formatLastSynced renders the absolute timestamp in the local zone, so pin the local zone to UTC
	// for deterministic expectations.
	origLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = origLocal })

	now := time.Unix(10_000, 0)
	fresh := now.Add(-30 * time.Second)
	stale := now.Add(-(daemonstate.StaleThreshold + time.Minute))
	syncedAt := now.Add(-5 * time.Minute)

	tests := []struct {
		name  string
		repo  string
		state *daemonstate.State
		want  string
	}{
		{
			name: "synced fresh",
			repo: "/repo",
			state: &daemonstate.State{Repos: []daemonstate.RepoStatus{
				{Repo: "/repo", Status: daemonstate.StatusRunning, UpdatedAt: fresh, LastSyncedAt: syncedAt},
			}},
			want: "synced 1970-01-01 02:41:40 (5m ago)",
		},
		{
			name: "never synced",
			repo: "/repo",
			state: &daemonstate.State{Repos: []daemonstate.RepoStatus{
				{Repo: "/repo", Status: daemonstate.StatusRunning, UpdatedAt: fresh},
			}},
			want: "never synced",
		},
		{
			name: "stale entry omits segment",
			repo: "/repo",
			state: &daemonstate.State{Repos: []daemonstate.RepoStatus{
				{Repo: "/repo", Status: daemonstate.StatusRunning, UpdatedAt: stale, LastSyncedAt: syncedAt},
			}},
			want: "",
		},
		{
			name:  "missing entry omits segment",
			repo:  "/repo",
			state: &daemonstate.State{},
			want:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := repoLastSynced(tc.repo, tc.state, now)
			assert.Equal(t, got, tc.want)
		})
	}
}

// @description    Verifies formatLastSynced across time buckets.
//
// Test_FormatLastSynced covers the zero time (never synced) and the second, minute, hour, and day
// relative buckets so the segment reads correctly across sync ages.
//
// @param           t  "test handle used for table-driven assertions"
func Test_FormatLastSynced(t *testing.T) {
	// formatLastSynced renders the absolute timestamp in the local zone, so pin the local zone to UTC
	// for deterministic expectations.
	origLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = origLocal })

	now := time.Unix(2_000_000_000, 0)

	tests := []struct {
		name string
		last time.Time
		want string
	}{
		{name: "zero", last: time.Time{}, want: "never synced"},
		{name: "seconds", last: now.Add(-30 * time.Second), want: "synced 2033-05-18 03:32:50 (30s ago)"},
		{name: "minutes", last: now.Add(-5 * time.Minute), want: "synced 2033-05-18 03:28:20 (5m ago)"},
		{name: "hours", last: now.Add(-3 * time.Hour), want: "synced 2033-05-18 00:33:20 (3h ago)"},
		{name: "days", last: now.Add(-4 * 24 * time.Hour), want: "synced 2033-05-14 03:33:20 (4d ago)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, formatLastSynced(tc.last, now), tc.want)
		})
	}
}

// @description    Verifies humanDuration bucket boundaries and compound formatting.
//
// Test_HumanDuration covers the under-a-minute seconds form, the compound "<D>d<H>h<M>m" form with
// seconds dropped, the 7-day cutoff that collapses to just "<D>d", and the zero duration.
//
// @param           t  "test handle used for table-driven assertions"
func Test_HumanDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "zero", d: 0, want: "0s"},
		{name: "negative clamped", d: -45 * time.Second, want: "0s"},
		{name: "seconds", d: 45 * time.Second, want: "45s"},
		{name: "just under a minute", d: 59*time.Second + 999*time.Millisecond, want: "59s"},
		{name: "minutes drop seconds", d: 5*time.Minute + 30*time.Second, want: "5m"},
		{name: "hours and minutes drop seconds", d: 2*time.Hour + 3*time.Minute + 4*time.Second, want: "2h3m"},
		{name: "days hours minutes", d: 1*24*time.Hour + 2*time.Hour + 3*time.Minute, want: "1d2h3m"},
		{name: "just under seven days stays compound", d: 6*24*time.Hour + 23*time.Hour, want: "6d23h"},
		{name: "seven days collapses", d: 7 * 24 * time.Hour, want: "7d"},
		{name: "over seven days drops finer units", d: 7*24*time.Hour + 4*time.Hour, want: "7d"},
		{name: "ten days", d: 10 * 24 * time.Hour, want: "10d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, humanDuration(tc.d), tc.want)
		})
	}
}

// @description    Verifies padRight rune-width padding.
//
// Test_PadRight covers ASCII padding, the already-wide no-op, and multi-byte rune alignment.
//
// @param           t  "test handle used for table-driven assertions"
func Test_PadRight(t *testing.T) {
	assert.Equal(t, padRight("ab", 5), "ab   ")
	assert.Equal(t, padRight("abcde", 3), "abcde")
	assert.Equal(t, padRight("世界", 4), "世界  ")
}

// @description    Verifies printRepos aligns columns and omits the sync segment for unknown rows.
//
// Test_PrintReposAlignsColumns captures the output of printRepos and verifies that the "  -  "
// separators line up across rows and that unknown-status rows omit the last-sync segment.
//
// @param           t  "test handle used for assertions"
func Test_PrintReposAlignsColumns(t *testing.T) {
	now := time.Unix(10_000, 0)
	fresh := now.Add(-30 * time.Second)
	state := &daemonstate.State{Repos: []daemonstate.RepoStatus{
		{Repo: "/a", Status: daemonstate.StatusRunning, UpdatedAt: fresh, LastSyncedAt: now.Add(-3 * time.Minute)},
		{Repo: "/longer-path", Status: daemonstate.StatusPaused, Stage: "rebase", UpdatedAt: fresh, LastSyncedAt: now.Add(-3 * time.Minute)},
		{Repo: "/missing", Status: daemonstate.StatusRunning, UpdatedAt: time.Time{}},
	}}

	var buf bytes.Buffer
	printRepos(&buf, []string{"/a", "/longer-path", "/missing"}, state, now, "")

	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	assert.Equal(t, len(lines), 3)

	// Every row's first "  -  " separator must start at the same column (path column aligned to the
	// longest path). The unknown row ends after the status column (no second separator).
	firstSep := strings.Index(lines[0], "  -  ")
	assert.Equal(t, firstSep, len("/longer-path"))
	assert.Equal(t, strings.Index(lines[1], "  -  "), firstSep)
	assert.Equal(t, strings.Index(lines[2], "  -  "), firstSep)

	// The synced segment must follow the status column on rows with a fresh entry...
	assert.Assert(t, strings.Contains(lines[0], "synced "), "running row should include a synced segment")
	assert.Assert(t, strings.Contains(lines[1], "synced "), "paused row should include a synced segment")
	// ...but not on the unknown (stale) row.
	assert.Assert(t, !strings.Contains(lines[2], "synced "), "unknown row should omit the synced segment")
	assert.Assert(t, strings.Contains(lines[2], "unknown (daemon may not be running)"), "unknown row should carry the status label")
}
