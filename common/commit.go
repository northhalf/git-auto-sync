package common

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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
func commit(repoConfig RepoConfig) error {
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

	_, err = GitCommand(repoConfig, []string{"commit", "-m", msg})
	if err != nil {
		return tracerr.Wrap(err)
	}

	return nil
}

// @description    Runs a Git subprocess.
//
// GitCommand runs the configured Git executable in the repository directory with the resolved
// environment, captures standard output and standard error, warns about an omitted SSH agent
// socket, and includes command details in errors.
//
// @param           repoConfig   "repository, executable, and environment configuration"
//
// @param           args         "arguments to pass to the Git executable"
//
// @return          bytes.Buffer "captured standard output, including output from a failed command"
//
// @return          error        "nil on success, or an error containing command and captured output details"
func GitCommand(repoConfig RepoConfig, args []string) (bytes.Buffer, error) {
	repoPath := repoConfig.RepoPath

	var outb, errb bytes.Buffer

	cmd := "git"
	if repoConfig.GitExec != "" {
		cmd = repoConfig.GitExec
	}

	statusCmd := exec.Command(cmd, args...)
	statusCmd.Dir = repoPath
	statusCmd.Stdout = &outb
	statusCmd.Stderr = &errb
	statusCmd.Env = toEnvString(repoConfig)
	err := statusCmd.Run()

	if hasEnvVariable(os.Environ(), "SSH_AUTH_SOCK") && !hasEnvVariable(repoConfig.Env, "SSH_AUTH_SOCK") {
		fmt.Println("WARNING: SSH_AUTH_SOCK env variable isn't being passed")
	}

	if err != nil {
		fullCmd := "git " + strings.Join(args, " ")
		err := tracerr.Errorf("%w: Command: %s\nEnv: %s\nStdOut: %s\nStdErr: %s", err, fullCmd, statusCmd.Env, outb.String(), errb.String())
		return outb, err
	}
	return outb, nil
}

// @description    Builds the Git subprocess environment.
//
// toEnvString builds the subprocess environment from configured entries and the current process
// HOME value.
//
// @param           repoConfig  "repository configuration containing explicit environment entries"
//
// @return          []string    "environment entries for the Git subprocess"
func toEnvString(repoConfig RepoConfig) []string {
	vals := repoConfig.Env
	vals = append(vals, repoConfig.Env...)

	for _, s := range os.Environ() {
		parts := strings.Split(s, "=")
		k := parts[0]
		if k == "HOME" {
			vals = append(vals, s)
		}
	}

	return vals
}

// @description    hasEnvVariable reports whether an environment entry has the requested key.
//
// @param           all   "environment entries in key=value form"
//
// @param           name  "environment variable name to find"
//
// @return          bool  "true when an entry with the requested name exists"
func hasEnvVariable(all []string, name string) bool {
	for _, s := range all {
		parts := strings.Split(s, "=")
		k := parts[0]
		if k == name {
			return true
		}
	}
	return false
}
