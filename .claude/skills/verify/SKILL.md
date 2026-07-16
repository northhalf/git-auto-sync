---
name: verify
description: Use when verifying a git-auto-sync change end-to-end before claiming it works, or when the generic verify process needs this project's build and launch recipe
---

# Verify git-auto-sync CLI

## Overview

Drive the real `git-auto-sync` binary against a throwaway repository to observe runtime behavior. This is the project's build-and-launch recipe that the generic `verify` process reads when it needs a handle.

## Build

Build into `.tmp/` so binaries in `bin/` stay untouched:

```bash
go build -o .tmp/git-auto-sync-verify .
```

## Drive

1. Create an isolated repository with `mktemp -d` and `git init` inside it.
2. Add the files, `.gitignore`, and Git identity the flow needs.
3. Run the binary from that repository so repository discovery follows the real CLI path.
4. Capture stdout, stderr, and exit status for both success and error cases.

## Constraints

- Remove the temporary repository when finished.
- Never run sync or write operations against a real user repository.
- `.tmp/` is gitignored, so the build artifact is disposable.
