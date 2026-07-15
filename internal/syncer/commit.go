package syncer

import (
	"github.com/northhalf/git-auto-sync/internal/config"
	"sort"
	"strings"

	"github.com/ztrue/tracerr"
	"gopkg.in/src-d/go-git.v4"
)

// @description    Commits eligible worktree changes.
//
// commit filters changed files through ShouldIgnoreFile, stages eligible files, sorts their status
// lines, and creates a commit with those lines as its message. It returns without committing when
// no eligible changes exist.
//
// @param           repoConfig  "configuration for the repository to commit"
//
// @return          error       "nil on success or no eligible changes, or a repository or Git error"
func commit(repoConfig config.RepoConfig) error {
	repoPath := repoConfig.RepoPath
	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return tracerr.Wrap(err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return tracerr.Wrap(err)
	}

	status, err := w.Status()
	if err != nil {
		return tracerr.Wrap(err)
	}

	hasChanges := false
	commitMsg := []string{}
	for filePath, fileStatus := range status {
		if fileStatus.Worktree == git.Unmodified && fileStatus.Staging == git.Unmodified {
			continue
		}

		ignore, err := ShouldIgnoreFile(repoPath, filePath)
		if err != nil {
			return tracerr.Wrap(err)
		}

		if ignore {
			continue
		}

		hasChanges = true
		_, err = w.Add(filePath)
		if err != nil {
			return tracerr.Wrap(err)
		}

		msg := ""
		if fileStatus.Worktree == git.Untracked && fileStatus.Staging == git.Untracked {
			msg += "?? "
		} else {
			msg += " " + string(fileStatus.Worktree) + " "
		}
		msg += filePath
		commitMsg = append(commitMsg, msg)
	}

	sort.Strings(commitMsg)
	msg := strings.Join(commitMsg, "\n")

	if !hasChanges {
		return nil
	}

	_, err = gitCommand(repoConfig, []string{"commit", "-m", msg})
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}
