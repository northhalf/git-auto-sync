package syncer

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
)

// @description    Runs a Git subprocess.
//
// gitCommand runs the configured Git executable in the repository directory with the resolved
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
func gitCommand(repoConfig config.RepoConfig, args []string) (bytes.Buffer, error) {
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
		slog.Warn("SSH_AUTH_SOCK env variable isn't being passed")
	}

	if err != nil {
		fullCmd := cmd + " " + strings.Join(args, " ")
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
func toEnvString(repoConfig config.RepoConfig) []string {
	vals := append([]string(nil), repoConfig.Env...)

	for _, s := range os.Environ() {
		k, _, _ := strings.Cut(s, "=")
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
		k, _, _ := strings.Cut(s, "=")
		if k == name {
			return true
		}
	}
	return false
}
