---
name: writing-commit-message
description: Use when the user requests to write a git commit message for the git-auto-sync project. Load this skill to generate the commit message and write it to .tmp/commit_message.md
---

# Git Commit Message Specification

## Title Format

```
<type>(<scope>): <English summary>
```

- **type**: `feat` | `fix` | `perf` | `refactor` | `docs` | `test`
- **scope**: one scope per module, matching the changed package or directory:
  - `cli` - the `git-auto-sync` CLI: root commands and repo validation (root `*.go`).
  - `syncer` - the sync pipeline: commit, fetch, rebase, push, and ignore (`internal/syncer`).
  - `watcher` - filesystem monitoring (`internal/watcher`).
  - `daemon` - the background service process and its install/start/stop lifecycle (`daemon/`, `internal/daemonservice`).
  - `config` - daemon and per-repository `[auto-sync]` settings (`internal/config`).
  - `logging` - CLI and daemon logging (`internal/logging`).
  - `build` - build, release, CI, dependencies, and packaging (`Makefile`, `.version`, `go.mod`, `.github/`, `assets/`, `completions/`).
  - `docs` - documentation (`README.md`, `README_zh.md`).
- **summary**: One sentence describing what was done, starting with a verb, in English.

When a change spans multiple modules, pick the scope of the most significant changed file.

Examples:
- `feat(daemon): reload daemon config without restart via config-file polling`
- `refactor(syncer): cache Git index and ignore rules in IgnoreChecker for each sync round`
- `fix(watcher): debounce file changes without delaying scheduled syncs`
- `docs: rewrite README, add Chinese README, and trim release config`

## Body Structure

The body is organized into the following paragraphs, each beginning with a bold label:

### Feature/Fix Description (first paragraph)

Briefly explain the purpose and core changes of this commit, in 2–4 sentences.

### Engineering Changes

Starts with **Engineering Changes:**, lists all added/modified/deleted files with specific changes:

```
Engineering Changes:
- Added internal/syncer/ignore_checker.go: IgnoreChecker struct caching the
  repository root, a tracked-path set, and a compiled gitignore matcher;
  NewIgnoreChecker reads the index and patterns once, ShouldIgnore reuses the
  snapshot per path
- Modified internal/syncer/commit.go: build one IgnoreChecker before the status
  loop and call checker.ShouldIgnore per record instead of ShouldIgnoreFile
```

### Tests

Starts with **Tests:**, describing test results:

```
Tests:
- Added internal/syncer/ignore_checker_test.go: 4 unit tests
- go test ./...: all packages pass; golangci-lint run ./...: 0 issues; gofmt clean
```

For documentation-only changes:
```
Tests:
- Documentation-only change, no code tests run
```

### Documentation

Starts with **Documentation:**, listing updated doc files.

**Exclusion rule**: Changes under `docs/superpowers/` are not written here (they belong to skill plugin docs, not project docs).

```
Documentation:
- Updated README.md: added .gitkeep to the ignored-files exceptions
- Updated README_zh.md: mirrored the same change in the 忽略文件 section
```

## Full Example

```
refactor(syncer): cache Git index and ignore rules in IgnoreChecker for each sync round

Extract a reusable IgnoreChecker from the per-path ShouldIgnoreFile so the
commit loop no longer re-opens the repository, re-reads the index, and
re-parses gitignore patterns for every file in a single sync round. One
checker is built before iterating the status output and reused across all
paths; ShouldIgnoreFile remains a single-shot convenience wrapper for the
check CLI.

Engineering Changes:
- Added internal/syncer/ignore_checker.go: IgnoreChecker caching the repo
  root, a tracked-path set from the Git index, and a compiled gitignore
  matcher; NewIgnoreChecker reads index and patterns once, ShouldIgnore
  reuses the snapshot so tracked files bypass every check
- Modified internal/syncer/ignore.go: ShouldIgnoreFile now wraps an
  IgnoreChecker; removed the unused isTracked and isFileIgnoredByGit helpers
- Modified internal/syncer/commit.go: build one IgnoreChecker before the
  status loop and call checker.ShouldIgnore per record

Tests:
- Added internal/syncer/ignore_checker_test.go: 4 unit tests covering
  parity with ShouldIgnoreFile, round reuse, tracked bypass, and non-repo
  construction failure
- go test ./...: all packages pass; golangci-lint run ./...: 0 issues; gofmt clean

Documentation:
- Updated README.md: added .gitkeep to the ignored-files exceptions
- Updated README_zh.md: mirrored the change in the 忽略文件 section
```

## Complete Workflow (Mandatory)

Before writing the commit message, you must follow this sequence:

### 1. Read project documentation for context

Read the project documentation to understand the current structure and conventions:

- `CLAUDE.md` - project architecture, build/test/lint commands, code style
- `README.md` / `README_zh.md` - features, usage, and deployment notes

### 2. Review recent commits as reference

```bash
git log -3 --format="%H%n%s%n%b%n---"   # last 3 full commits
```

Refer to historical type selection, summary style, and body structure. Choose the scope from the project's modules (see the scope list above), not from past commit subjects.

### 3. Determine the scope of changes with git diff

```bash
git diff --name-only          # working tree changes
git diff --cached --name-only # staged changes
```

Classify changed files by module to choose the scope: `cli` (root `*.go`), `syncer`/`watcher`/`config`/`logging` (`internal/...`), `daemon` (`daemon/`, `internal/daemonservice`), `build` (`Makefile`, `.version`, `go.mod`, `.github/`, `assets/`, `completions/`), `docs` (`README.md`, `README_zh.md`).

### 4. Analyze the role of the changes in the project

- Determine change type: feat / fix / perf / refactor / docs / test
- Determine scope from the changed package or directory (see the scope list above); when multiple modules change, use the most significant one.
- Understand its position and role in the overall project architecture (which module, what problem it solves, what improvement it brings)

### 5. Generate commit message

Based on the analysis above, generate the commit message according to this specification.

## Output Method (Mandatory)

After generating the commit message, you **must** write it to the `.tmp/commit_message.md` file, **overwriting** (not appending).

Process:
1. `mkdir -p .tmp` (ensure directory exists)
2. Use Write tool to write the full commit message into `.tmp/commit_message.md` (overwrite each time)

**Prohibited**: Do not output the commit message in the terminal, do not invoke `git commit` directly. Only write the file.

The user will review `.tmp/commit_message.md` and commit manually.

## Prohibited Actions

- Do not run `git add` or `git commit` on your own; the commit must be reviewed by the user.
- Do not use non-English in the title (except type/scope).
- Do not omit the body structure paragraph labels.
