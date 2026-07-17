package syncer

import (
	"bytes"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
)

// @description    Runs a Git subprocess.
//
// gitCommand runs the configured Git executable in the repository directory with the resolved
// environment, captures standard output and standard error, and includes command details in
// returned errors. Structured log attributes identify the operation without separately exposing
// environment values, command output, or remote URLs.
//
// @param           logger       "repository-scoped logger"
//
// @param           repoConfig   "repository, executable, and environment configuration"
//
// @param           args         "arguments to pass to the Git executable"
//
// @return          bytes.Buffer "captured standard output, including output from a failed command"
//
// @return          error        "nil on success, or an error containing command and captured output details"
func gitCommand(logger *slog.Logger, repoConfig config.RepoConfig, args []string) (bytes.Buffer, error) {
	operation := "unknown"
	if len(args) > 0 {
		operation = args[0]
	}
	logger.Debug("starting git command", "operation", operation)

	var outb, errb bytes.Buffer

	cmd := "git"
	if repoConfig.GitExec != "" {
		cmd = repoConfig.GitExec
	}

	// Prepend core.quotePath=false so Git emits raw UTF-8 path bytes instead of octal C-style
	// escapes such as "\346\265\213". The -z flag used by status already disables quoting, but
	// setting this explicitly keeps path output parseable for non-ASCII filenames regardless of
	// the user's git config, and makes non-status command output (fetch, rebase, ...) readable.
	gitArgs := []string{"-c", "core.quotePath=false"}
	if runtime.GOOS == "windows" {
		// The daemon service runs as LocalSystem while repositories are owned by the installing
		// user, so git's safe.directory ownership check refuses worktree operations on them. Trust
		// this repository for the command so the daemon can sync user-owned repos without requiring
		// a global safe.directory entry that would also relax the user's interactive git. Git on
		// Windows compares safe.directory paths using forward slashes, so normalize the repo path.
		gitArgs = append(gitArgs, "-c", "safe.directory="+filepath.ToSlash(repoConfig.RepoPath))
	}
	gitArgs = append(gitArgs, args...)

	statusCmd := exec.Command(cmd, gitArgs...)
	statusCmd.Dir = repoConfig.RepoPath
	statusCmd.Stdout = &outb
	statusCmd.Stderr = &errb
	statusCmd.Env = toEnvString(repoConfig)
	err := statusCmd.Run()

	if err != nil {
		fullCmd := cmd + " " + strings.Join(gitArgs, " ")
		// Expose only environment keys, not values, so secrets such as tokens or agent sockets
		// passed through repoConfig.Env or the inherited parent environment never reach logs.
		keys := make([]string, 0, len(statusCmd.Env))
		for _, e := range statusCmd.Env {
			k, _, _ := strings.Cut(e, "=")
			keys = append(keys, k)
		}
		return outb, tracerr.Errorf("%w: Command: %s\nEnv: %s\nStdOut: %s\nStdErr: %s", err, fullCmd, keys, outb.String(), errb.String())
	}

	logger.Debug("git command completed", "operation", operation)
	return outb, nil
}

// @description    Builds the Git subprocess environment.
//
// toEnvString inherits the full parent environment so Git receives SSH_AUTH_SOCK, PATH,
// XDG_CONFIG_HOME, GIT_* and any other variable it relies on, then layers explicit per-repo
// entries on top so configured values override inherited ones. Entries are sorted for stable
// command-error output.
//
// @param           repoConfig  "repository configuration containing explicit environment entries"
//
// @return          []string    "environment entries for the Git subprocess"
func toEnvString(repoConfig config.RepoConfig) []string {
	env := envMapFromSlice(os.Environ())
	for k, v := range envMapFromSlice(repoConfig.Env) {
		env[k] = v
	}

	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}
