package syncer

import (
	"github.com/northhalf/git-auto-sync/internal/config"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"gotest.tools/v3/assert"
)

// @description    Prepares a repository test fixture.
//
// PrepareFixture creates a temporary repository, copies the named fixture into it, renames the
// fixture's .gitted directory to .git, and returns its repository configuration.
//
// @param           t      "test handle used to fail on fixture preparation errors"
//
// @param           name   "name of the fixture directory under testdata"
//
// @return          config.RepoConfig   "configuration for the copied temporary repository"
func PrepareFixture(t *testing.T, name string) config.RepoConfig {
	newRepoPath, err := os.MkdirTemp(os.TempDir(), name)
	assert.NilError(t, err)

	fixturePath := filepath.Join("testdata", name)
	err = os.CopyFS(newRepoPath, os.DirFS(fixturePath))
	assert.NilError(t, err)

	err = os.Rename(filepath.Join(newRepoPath, ".gitted"), filepath.Join(newRepoPath, ".git"))
	assert.NilError(t, err)

	repoConfig, err := config.NewRepoConfig(newRepoPath)
	assert.NilError(t, err)

	return repoConfig
}

// @description    Verifies that unchanged fixtures create no commits.
//
// Test_NoChanges verifies that committing an unchanged fixture succeeds without changing the
// repository's HEAD commit.
//
// @param           t   "test handle used for fixture setup and assertions"
func Test_NoChanges(t *testing.T) {
	repoConfig := PrepareFixture(t, "no_changes")

	err := commit(slog.Default(), repoConfig)
	assert.NilError(t, err)

	r, err := git.PlainOpen(repoConfig.RepoPath)
	assert.NilError(t, err)

	head, err := r.Head()
	assert.NilError(t, err)

	assert.Equal(t, head.Hash(), plumbing.NewHash("28cc969d97ddb7640f5e1428bbc8f2947d1ffd57"))
}

// @description    Checks the repository HEAD commit.
//
// HasHeadCommit opens a Git repository and asserts that HEAD is a new commit whose parent has the
// expected hash and whose message matches the expected text.
//
// @param           t          "test handle used for Git-operation and commit assertions"
//
// @param           repoPath   "path to the repository under test"
//
// @param           hash       "expected hash of the HEAD commit's parent"
//
// @param           msg        "expected message of the HEAD commit"
func HasHeadCommit(t *testing.T, repoPath string, hash string, msg string) {
	r, err := git.PlainOpen(repoPath)
	assert.NilError(t, err)

	head, err := r.Head()
	assert.NilError(t, err)

	assert.Assert(t, head.Hash() != plumbing.NewHash(hash))

	commit, err := r.CommitObject(head.Hash())
	assert.NilError(t, err)

	parent, err := commit.Parent(0)
	assert.NilError(t, err)
	assert.Equal(t, parent.ID(), plumbing.NewHash(hash))
	assert.Equal(t, commit.Message, msg)
}

// @description    Verifies that LFS-tracked files matching their pointer are not committed.
//
// Test_LFSNoChanges verifies that a working-tree binary whose index entry is a Git LFS pointer is
// treated as unchanged and does not produce a new commit.
//
// @param           t   "test handle used for fixture setup and assertions"
func Test_LFSNoChanges(t *testing.T) {
	repoConfig := PrepareFixture(t, "lfs_no_changes")

	err := commit(slog.Default(), repoConfig)
	assert.NilError(t, err)

	r, err := git.PlainOpen(repoConfig.RepoPath)
	assert.NilError(t, err)

	head, err := r.Head()
	assert.NilError(t, err)

	assert.Equal(t, head.Hash(), plumbing.NewHash("3817fd1942f3ab9960a0baeb3503cfbcb7f6e1fe"))
}

// @description    Verifies that an LFS pointer left in the working tree is not committed.
//
// Test_LFSPointerInWorktree verifies that a Git LFS file whose working-tree content is the pointer
// text (as a CI checkout without an LFS fetch or a GIT_LFS_SKIP_SMUDGE environment leaves it) is
// treated as unchanged: git status reports it as modified but git add stages nothing, so commit
// skips rather than failing with "nothing to commit".
//
// @param           t   "test handle used for fixture setup and assertions"
func Test_LFSPointerInWorktree(t *testing.T) {
	repoConfig := PrepareFixture(t, "lfs_no_changes")

	// Replace the smudged binary with the pointer text stored in the index, mirroring a working
	// tree populated by an LFS-less checkout.
	pointer, err := exec.Command("git", "-C", repoConfig.RepoPath, "cat-file", "-p", ":image.bin").Output()
	assert.NilError(t, err)
	assert.NilError(t, os.WriteFile(filepath.Join(repoConfig.RepoPath, "image.bin"), pointer, 0o644))

	err = commit(slog.Default(), repoConfig)
	assert.NilError(t, err)

	r, err := git.PlainOpen(repoConfig.RepoPath)
	assert.NilError(t, err)
	head, err := r.Head()
	assert.NilError(t, err)
	assert.Equal(t, head.Hash(), plumbing.NewHash("3817fd1942f3ab9960a0baeb3503cfbcb7f6e1fe"))
}

// @description    Verifies commits for untracked files.
//
// Test_NewFile verifies that committing an untracked file creates a new HEAD commit with the
// original HEAD as its parent and the untracked-file status as its message.
//
// @param           t   "test handle used for fixture setup and assertions"
func Test_NewFile(t *testing.T) {
	repoConfig := PrepareFixture(t, "new_file")

	err := commit(slog.Default(), repoConfig)
	assert.NilError(t, err)

	HasHeadCommit(t, repoConfig.RepoPath, "28cc969d97ddb7640f5e1428bbc8f2947d1ffd57", "?? 2.md\n")
}

// @description    Verifies commits for a modified file.
//
// Test_OneFileChange verifies that committing one modified file creates a new HEAD commit with the
// original HEAD as its parent and the modified-file status as its message.
//
// @param           t   "test handle used for fixture setup and assertions"
func Test_OneFileChange(t *testing.T) {
	repoConfig := PrepareFixture(t, "one_file_change")

	err := commit(slog.Default(), repoConfig)
	assert.NilError(t, err)

	HasHeadCommit(t, repoConfig.RepoPath, "28cc969d97ddb7640f5e1428bbc8f2947d1ffd57", " M 1.md\n")
}

// @description    Verifies that Vim swap files are ignored.
//
// Test_VimSwapFile verifies that committing when the only working-tree change is a Vim swap file
// succeeds without changing the repository's HEAD commit.
//
// @param           t   "test handle used for fixture setup and assertions"
func Test_VimSwapFile(t *testing.T) {
	repoConfig := PrepareFixture(t, "vim_swap_file")

	err := commit(slog.Default(), repoConfig)
	assert.NilError(t, err)

	r, err := git.PlainOpen(repoConfig.RepoPath)
	assert.NilError(t, err)

	head, err := r.Head()
	assert.NilError(t, err)

	assert.Equal(t, head.Hash(), plumbing.NewHash("28cc969d97ddb7640f5e1428bbc8f2947d1ffd57"))
}

// @description    Verifies that nested Git repositories are not committed as gitlinks.
//
// Test_NestedGitRepositorySkipped verifies that a linked worktree created under the worktree (as
// .claude/worktrees/ is) is not staged or committed as an embedded gitlink, while a regular
// untracked file alongside it is committed normally.
//
// @param           t   "test handle used for fixture setup and assertions"
func Test_NestedGitRepositorySkipped(t *testing.T) {
	repoConfig := PrepareFixture(t, "no_changes")

	// Create a linked worktree inside the repository, mirroring .claude/worktrees/.
	worktreeCmd := exec.Command("git", "-C", repoConfig.RepoPath, "worktree", "add", "--detach", ".claude/worktrees/feat")
	assert.NilError(t, worktreeCmd.Run())

	// Add a legitimate untracked file in the parent worktree.
	assert.NilError(t, os.WriteFile(filepath.Join(repoConfig.RepoPath, "real.md"), []byte("real"), 0o644))

	err := commit(slog.Default(), repoConfig)
	assert.NilError(t, err)

	// The legitimate file must be committed with its status line as the message.
	r, err := git.PlainOpen(repoConfig.RepoPath)
	assert.NilError(t, err)
	head, err := r.Head()
	assert.NilError(t, err)
	c, err := r.CommitObject(head.Hash())
	assert.NilError(t, err)
	assert.Equal(t, c.Message, "?? real.md\n")

	// The linked worktree must NOT be tracked as an embedded gitlink.
	tracked, err := exec.Command("git", "-C", repoConfig.RepoPath, "ls-files", ".claude/worktrees/feat").Output()
	assert.NilError(t, err)
	assert.Equal(t, string(tracked), "")
}

// @description    Verifies that non-ASCII filenames are committed with raw UTF-8 bytes.
//
// Test_NonASCIIFilename verifies that a file whose name contains Chinese characters and a space is
// staged and committed using its raw path, not Git's octal C-style escaping. The -z status flag
// together with core.quotePath=false keeps path bytes literal so git add and the commit message
// carry the real filename.
//
// @param           t   "test handle used for fixture setup and assertions"
func Test_NonASCIIFilename(t *testing.T) {
	repoConfig := PrepareFixture(t, "no_changes")

	assert.NilError(t, os.WriteFile(filepath.Join(repoConfig.RepoPath, "新建 文档.md"), []byte("content"), 0o644))

	err := commit(slog.Default(), repoConfig)
	assert.NilError(t, err)

	r, err := git.PlainOpen(repoConfig.RepoPath)
	assert.NilError(t, err)
	head, err := r.Head()
	assert.NilError(t, err)
	c, err := r.CommitObject(head.Hash())
	assert.NilError(t, err)
	assert.Equal(t, c.Message, "?? 新建 文档.md\n")

	// The file must be tracked under its raw UTF-8 path.
	tracked, err := exec.Command("git", "-C", repoConfig.RepoPath, "-c", "core.quotePath=false", "ls-files", "-z", "--", "新建 文档.md").Output()
	assert.NilError(t, err)
	assert.Equal(t, string(tracked), "新建 文档.md\x00")
}

// @description    Verifies commits for multiple file changes.
//
// Test_MultipleFileChange verifies that committing deleted, modified, and untracked files creates
// a new HEAD commit with the expected parent and multiline status message.
//
// @param           t   "test handle used for fixture setup and assertions"
func Test_MultipleFileChange(t *testing.T) {
	repoConfig := PrepareFixture(t, "multiple_file_change")

	err := commit(slog.Default(), repoConfig)
	assert.NilError(t, err)

	HasHeadCommit(t, repoConfig.RepoPath, "7058b6b292ee3d1382670334b5f29570a1117ef1", ` D dirA/2.md
 M 1.md
?? dirB/3.md
`)
}
