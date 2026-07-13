---
name: writing-commit-message
description: Use when the user requests to write a git commit message for the meme-pilot project. Load this skill to generate the commit message and write it to .tmp/commit_message.md
---

# Git Commit Message Specification

## Title Format

```
<type>(<scope>): <English summary>
```

- **type**: `feat` | `fix` | `perf` | `refactor` | `docs` | `test`
- **scope**: `engine` | `plugins` | `docs` | `config` | `integration`
- **summary**: One sentence describing what was done, starting with a verb, in English.

Examples:
- `feat(engine): implement lossless image compression module image_optimizer`
- `refactor(engine): extract shared protocols, package-level exports and relative import restructuring`
- `docs: fix cross-document inconsistencies (model names, env vars, Python version)`

## Body Structure

The body is organized into the following paragraphs, each beginning with a bold label:

### Feature/Fix Description (first paragraph)

Briefly explain the purpose and core changes of this commit, in 2–4 sentences.

### Engineering Changes

Starts with **Engineering Changes:**, lists all added/modified/deleted files with specific changes:

```
Engineering Changes:
- Added bot/engine/image_optimizer.py: ImageOptimizer class + OptimizeResult
  dataclass; JPEG EXIF metadata removal + high-quality re-encoding (quality=95)...
- Modified bot/engine/__init__.py: export ImageOptimizer and OptimizeResult
- Modified bot/engine/index_manager.py: __init__ added optimizer parameter
```

### Tests

Starts with **Tests:**, describing test results:

```
Tests:
- Added tests/unit/engine/test_image_optimizer.py: 11 unit tests
- uv run pytest: 237 passed (215 unit + 22 integration)
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
- Updated docs/process.md: appended image_optimizer.py completion record
- Updated docs/api/API.md: added image_optimizer section to index
```

## Full Example

```
feat(engine): implement lossless image compression module image_optimizer

Implement bot/engine/image_optimizer.py to perform lossless compression
on meme images, reducing file size before indexing. Uses Pillow for
JPEG/PNG/WebP/GIF formats, BMP is skipped.

Engineering Changes:
- Added bot/engine/image_optimizer.py: ImageOptimizer class + OptimizeResult
  dataclass; JPEG: EXIF metadata removal + high quality re-encoding (quality=95,
  optimize=True, progressive=True), PNG: optimize=True truly lossless,
  WebP: lossless mode (quality=80, method=6), GIF: preserve animation
  properties (duration/loop/transparency) remove redundant metadata;
  BMP: skipped returns skipped=True; atomic write (.tmp + os.replace),
  keep original file if compressed size larger (returns skipped=True)
- Added Pillow production dependency (uv add Pillow, v12.2.0)
- Modified bot/engine/__init__.py: export ImageOptimizer and OptimizeResult

Tests:
- Added tests/unit/engine/test_image_optimizer.py: 11 unit tests,
  covering OptimizeResult creation/frozen/skipped, ValueError for
  unsupported format, BMP skip, file not found, JPEG compression/EXIF
  removal, PNG/WebP/GIF compression, GIF animation preservation
- uv run pytest: 237 passed (215 unit + 22 integration)

Documentation:
- Added docs/api/bot/engine/image_optimizer.md: interface documentation
- Updated docs/api/API.md: added image_optimizer section to index
- Updated docs/process.md: appended completion record
```

## Complete Workflow (Mandatory)

Before writing the commit message, you must follow this sequence:

### 1. Read project documentation for context

Read the files specified in `CLAUDE.md` “required documents” to understand the current project structure and conventions:

- `docs/PRD.md` — requirements and feature boundaries
- `CONTEXT.md` — terminology and domain concepts
- `README.md` / `.env.example` / `docker-compose.yml` — deployment and environment
- `docs/api/API.md` — existing module interfaces

### 2. Review recent commits as reference

```bash
git log -3 --format="%H%n%s%n%b%n---"   # last 3 full commits
```

Refer to historical type/scope selection, summary style, and body structure.

### 3. Determine the scope of changes with git diff

```bash
git diff --name-only          # working tree changes
git diff --cached --name-only # staged changes
```

Classify changed files: code (`bot/`, `tests/`), config (root dir), documentation (`docs/`).

### 4. Analyze the role of the changes in the project

- Determine change type: feat / fix / perf / refactor / docs / test
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