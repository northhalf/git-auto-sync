package syncer

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/northhalf/git-auto-sync/internal/config"
)

func TestEnsureGitAuthor_LocalConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "")

	repoPath := filepath.Join(tmp, "repo")

	_, err := git.PlainInit(repoPath, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}

	if out, err := runGit(t, repoPath, "config", "user.email", "local@example.com"); err != nil {
		t.Fatalf("set user.email: %v\n%s", err, out)
	}
	if out, err := runGit(t, repoPath, "config", "user.name", "Local User"); err != nil {
		t.Fatalf("set user.name: %v\n%s", err, out)
	}

	err = ensureGitAuthor(slog.Default(), config.RepoConfig{RepoPath: repoPath})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestEnsureGitAuthor_GlobalConfig(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "")

	repoPath := filepath.Join(tmp, "repo")
	if _, err := git.PlainInit(repoPath, false); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	gitconfigPath := filepath.Join(home, ".gitconfig")
	gitconfigContent := "[user]\n\temail = global@example.com\n\tname = Global User\n"
	if err := os.WriteFile(gitconfigPath, []byte(gitconfigContent), 0o644); err != nil {
		t.Fatalf("write global .gitconfig: %v", err)
	}

	err := ensureGitAuthor(slog.Default(), config.RepoConfig{RepoPath: repoPath})
	if err != nil {
		t.Fatalf("expected success from global config, got %v", err)
	}
}

func TestEnsureGitAuthor_EnvOverride(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "")

	altHome := filepath.Join(tmp, "althome")
	if err := os.MkdirAll(altHome, 0o755); err != nil {
		t.Fatalf("create alt home: %v", err)
	}
	gitconfigPath := filepath.Join(altHome, ".gitconfig")
	gitconfigContent := "[user]\n\temail = alt@example.com\n\tname = Alt User\n"
	if err := os.WriteFile(gitconfigPath, []byte(gitconfigContent), 0o644); err != nil {
		t.Fatalf("write alt .gitconfig: %v", err)
	}

	repoPath := filepath.Join(tmp, "repo")
	if _, err := git.PlainInit(repoPath, false); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	err := ensureGitAuthor(slog.Default(), config.RepoConfig{
		RepoPath: repoPath,
		Env:      []string{"HOME=" + altHome},
	})
	if err != nil {
		t.Fatalf("expected success from env HOME override, got %v", err)
	}

	if got := os.Getenv("HOME"); got != home {
		t.Fatalf("HOME not restored: expected %q, got %q", home, got)
	}
}

func TestEnsureGitAuthor_MissingAuthor(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create home: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("GIT_CONFIG_GLOBAL", "")

	if out, err := runGit(t, tmp, "config", "--system", "user.email"); err == nil && strings.TrimSpace(out) != "" {
		t.Skip("system git config has user.email")
	}

	repoPath := filepath.Join(tmp, "repo")
	if _, err := git.PlainInit(repoPath, false); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	err := ensureGitAuthor(slog.Default(), config.RepoConfig{RepoPath: repoPath})
	if !errors.Is(err, errNoGitAuthorEmail) {
		t.Fatalf("expected errNoGitAuthorEmail, got %v", err)
	}
}

func runGit(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
