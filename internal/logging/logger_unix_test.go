//go:build unix

package logging

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const loggerFIFOHelperEnv = "GIT_AUTO_SYNC_LOGGER_FIFO_HELPER"

// @description    Verifies logger setup promptly falls back when the log path is a FIFO.
//
// TestSetupLoggerWithPathFallsBackPromptlyForFIFO runs setup in a subprocess so a regression that
// blocks while opening the FIFO can be killed without leaving a stuck goroutine in the test process.
//
// @param           t  "test handle used for FIFO creation and subprocess assertions"
func TestSetupLoggerWithPathFallsBackPromptlyForFIFO(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "git-auto-sync.log")
	if err := syscall.Mkfifo(logPath, 0o600); err != nil {
		t.Fatalf("Mkfifo(%q) error = %v", logPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSetupLoggerFIFOHelper$")
	cmd.Env = append(os.Environ(), loggerFIFOHelperEnv+"="+logPath)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("logger setup blocked on FIFO for longer than 2s; output: %s", output)
	}
	if err != nil {
		t.Fatalf("FIFO helper failed: %v; output: %s", err, output)
	}
}

// @description    Runs logger setup against the FIFO supplied by the parent regression test.
//
// @param           t  "test handle used for fallback assertions"
func TestSetupLoggerFIFOHelper(t *testing.T) {
	logPath := os.Getenv(loggerFIFOHelperEnv)
	if logPath == "" {
		t.Skip("helper subprocess only")
	}

	logger, logCloser := setupLoggerWithPath(false, logPath)
	if logger == nil {
		t.Fatal("setupLoggerWithPath() fallback logger is nil")
	}
	if logCloser != nil {
		t.Fatal("setupLoggerWithPath() fallback closer is nonnil")
	}
}
