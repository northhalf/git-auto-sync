package main

import (
	"testing"
	"time"

	"github.com/northhalf/git-auto-sync/internal/daemonstate"
	"gotest.tools/v3/assert"
)

// @description    Verifies repository status text formatting.
//
// Test_RepoStatusText covers the running, paused-with-reason, stale, and missing-entry cases so the
// ls and status output carries the expected label for each watcher state.
//
// @param           t  "test handle used for table-driven assertions"
func Test_RepoStatusText(t *testing.T) {
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
			got := repoStatusText(tc.repo, tc.state, now)
			assert.Equal(t, got, tc.want)
		})
	}
}
