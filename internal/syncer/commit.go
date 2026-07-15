package syncer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/northhalf/git-auto-sync/internal/config"
	"github.com/ztrue/tracerr"
)

// @description    Commits eligible worktree changes.
//
// commit filters changed files through ShouldIgnoreFile, stages eligible files, sorts their status
// lines, and creates a commit with those lines as its message. It logs a skip when no eligible
// changes exist.
//
// @param           logger      "repository-scoped logger"
//
// @param           repoConfig  "configuration for the repository to commit"
//
// @return          error       "nil on success or no eligible changes, or a repository or Git error"
func commit(logger *slog.Logger, repoConfig config.RepoConfig) error {
	logger.Debug("starting commit")

	repoPath := repoConfig.RepoPath
	repo, err := git.PlainOpenWithOptions(repoPath, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		logger.Error("commit failed", "operation", "open repository", "error", err)
		return tracerr.Wrap(err)
	}

	w, err := repo.Worktree()
	if err != nil {
		logger.Error("commit failed", "operation", "open worktree", "error", err)
		return tracerr.Wrap(err)
	}

	status, err := w.Status()
	if err != nil {
		logger.Error("commit failed", "operation", "read status", "error", err)
		return tracerr.Wrap(err)
	}

	idx, err := repo.Storer.Index()
	if err != nil {
		logger.Error("commit failed", "operation", "read index", "error", err)
		return tracerr.Wrap(err)
	}

	hasChanges := false
	commitMsg := []string{}
	for filePath, fileStatus := range status {
		if fileStatus.Worktree == git.Unmodified && fileStatus.Staging == git.Unmodified {
			continue
		}

		if isUnchangedLFSPointer(idx, repo, repoPath, filePath) {
			logger.Debug("commit skipping unchanged LFS file", "path", filePath)
			continue
		}

		ignore, err := ShouldIgnoreFile(repoPath, filePath)
		if err != nil {
			logger.Error("commit failed", "operation", "check ignored file", "path", filePath, "error", err)
			return tracerr.Wrap(err)
		}

		if ignore {
			continue
		}

		hasChanges = true
		_, err = w.Add(filePath)
		if err != nil {
			logger.Error("commit failed", "operation", "stage file", "path", filePath, "error", err)
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
		logger.Info("commit skipped", "reason", "no eligible changes")
		return nil
	}

	_, err = gitCommand(logger, repoConfig, []string{"commit", "-m", msg})
	if err != nil {
		logger.Error("commit failed", "operation", "create commit", "error", err)
		return tracerr.Wrap(err)
	}

	logger.Info("commit completed", "msgs", strings.Join(commitMsg, " "))
	return nil
}

// @description    Checks whether a modified file is an unchanged Git LFS pointer.
//
// isUnchangedLFSPointer reports true when the index stores an LFS pointer and the working-tree
// file's computed pointer matches it exactly, meaning the real file content has not changed.
//
// @param           idx       "repository index containing the staged blob"
//
// @param           repo      "open go-git repository"
//
// @param           repoPath  "path to the repository root"
//
// @param           filePath  "repository-relative file path"
//
// @return          bool      "true when the file is an unchanged LFS pointer"
func isUnchangedLFSPointer(idx *index.Index, repo *git.Repository, repoPath string, filePath string) bool {
	entry, err := idx.Entry(filePath)
	if err != nil {
		return false
	}

	blob, err := repo.BlobObject(entry.Hash)
	if err != nil {
		return false
	}

	reader, err := blob.Reader()
	if err != nil {
		return false
	}
	defer func() { _ = reader.Close() }()

	indexContent, err := io.ReadAll(reader)
	if err != nil {
		return false
	}

	if !isLFSPointer(indexContent) {
		return false
	}

	computed, err := computeLFSPointer(filepath.Join(repoPath, filePath))
	if err != nil {
		return false
	}

	return bytes.Equal(indexContent, computed)
}

// @description    Reports whether content looks like a Git LFS pointer.
//
// @param           content  "blob bytes to inspect"
//
// @return          bool     "true when content starts with the LFS pointer version line"
func isLFSPointer(content []byte) bool {
	return bytes.HasPrefix(content, []byte("version https://git-lfs.github.com/spec/v1"))
}

// @description    Computes the Git LFS pointer for a working-tree file.
//
// @param           filePath  "absolute path to the working-tree file"
//
// @return          []byte  "formatted LFS pointer bytes"
//
// @return          error   "nil on success, or a file read error"
func computeLFSPointer(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, tracerr.Wrap(err)
	}

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	closeErr := file.Close()
	if err != nil {
		return nil, tracerr.Wrap(err)
	}
	if closeErr != nil {
		return nil, tracerr.Wrap(closeErr)
	}

	pointer := fmt.Sprintf(
		"version https://git-lfs.github.com/spec/v1\noid sha256:%s\nsize %d\n",
		hex.EncodeToString(hash.Sum(nil)),
		size,
	)
	return []byte(pointer), nil
}
