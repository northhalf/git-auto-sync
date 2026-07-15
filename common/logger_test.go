package common

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// @description    Verifies that a multi-handler is enabled when any child accepts a level.
//
// @param           t  "test handle used for level checks"
func TestMultiHandlerEnabled(t *testing.T) {
	tests := []struct {
		name     string
		level    slog.Level
		handlers []slog.Handler
		want     bool
	}{
		{
			name:  "no children",
			level: slog.LevelInfo,
			want:  false,
		},
		{
			name:  "all children disabled",
			level: slog.LevelDebug,
			handlers: []slog.Handler{
				slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo}),
				slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}),
			},
			want: false,
		},
		{
			name:  "one child enabled",
			level: slog.LevelInfo,
			handlers: []slog.Handler{
				slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}),
				slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &multiHandler{handlers: tt.handlers}
			if got := handler.Enabled(context.Background(), tt.level); got != tt.want {
				t.Fatalf("Enabled(%s) = %t, want %t", tt.level, got, tt.want)
			}
		})
	}
}

// @description    Verifies dispatch, record attributes, disabled-child filtering, and first-error handling.
//
// @param           t  "test handle used for dispatch assertions"
func TestMultiHandlerHandle(t *testing.T) {
	firstErr := errors.New("first handler failed")
	secondErr := errors.New("second handler failed")
	first := newRecordingHandler(slog.LevelDebug, firstErr)
	disabled := newRecordingHandler(slog.LevelError, nil)
	second := newRecordingHandler(slog.LevelDebug, secondErr)
	last := newRecordingHandler(slog.LevelDebug, nil)
	handler := &multiHandler{handlers: []slog.Handler{first, disabled, second, last}}

	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "sync complete", 0)
	record.AddAttrs(slog.String("repo", "example"), slog.Int("changes", 2))
	err := handler.Handle(context.Background(), record)

	if !errors.Is(err, firstErr) {
		t.Fatalf("Handle() error = %v, want first error %v", err, firstErr)
	}
	if disabled.callCount() != 0 {
		t.Fatalf("disabled handler called %d times, want 0", disabled.callCount())
	}
	for name, child := range map[string]*recordingHandler{
		"first":  first,
		"second": second,
		"last":   last,
	} {
		if child.callCount() != 1 {
			t.Fatalf("%s handler called %d times, want 1", name, child.callCount())
		}
		if got := child.output(); !strings.Contains(got, "sync complete") ||
			!strings.Contains(got, "repo=example") || !strings.Contains(got, "changes=2") {
			t.Errorf("%s handler output %q does not preserve record and attributes", name, got)
		}
	}
}

// @description    Verifies that derived attributes and groups reach every child without losing output.
//
// @param           t  "test handle used for derived-handler assertions"
func TestMultiHandlerWithAttrsAndGroup(t *testing.T) {
	var first bytes.Buffer
	var second bytes.Buffer
	base := &multiHandler{handlers: []slog.Handler{
		slog.NewTextHandler(&first, nil),
		slog.NewTextHandler(&second, nil),
	}}
	handler := base.WithAttrs([]slog.Attr{slog.String("component", "sync")}).WithGroup("request")

	slog.New(handler).Info("served", "status", 200)

	for name, output := range map[string]string{"first": first.String(), "second": second.String()} {
		for _, substring := range []string{"msg=served", "component=sync", "request.status=200"} {
			if !strings.Contains(output, substring) {
				t.Errorf("%s handler output %q does not contain %q", name, output, substring)
			}
		}
	}
}

// @description    Verifies file setup installs a default logger that writes DEBUG-and-higher records.
//
// @param           t  "test handle used for temporary file and global logger cleanup"
func TestSetupLoggerWithPathWritesDebugToFile(t *testing.T) {
	restoreDefaultLogger(t)
	logPath := filepath.Join(t.TempDir(), "nested", "git-auto-sync.log")

	logger, logCloser, err := setupLoggerWithPathAndOutput(false, logPath, io.Discard)
	if err != nil {
		t.Fatalf("setupLoggerWithPathAndOutput() error = %v, want nil", err)
	}
	if logCloser == nil {
		t.Fatal("setupLoggerWithPathAndOutput() closer is nil")
	}
	t.Cleanup(func() {
		if closeErr := logCloser.Close(); closeErr != nil {
			t.Errorf("closing log writer: %v", closeErr)
		}
	})
	if logger == nil {
		t.Fatal("setupLoggerWithPath() logger is nil")
	}
	if slog.Default() != logger {
		t.Fatal("setupLoggerWithPath() did not install returned logger as default")
	}
	logger.Debug("debug record", "repo", "example")
	logger.Info("info record")

	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", logPath, err)
	}
	output := string(contents)
	for _, substring := range []string{"level=DEBUG", "msg=\"debug record\"", "repo=example", "level=INFO", "msg=\"info record\""} {
		if !strings.Contains(output, substring) {
			t.Errorf("log output %q does not contain %q", output, substring)
		}
	}
}

// @description    Verifies setup creates a new Unix log file with owner-only permissions.
//
// @param           t  "test handle used for temporary file and mode assertions"
func TestSetupLoggerWithPathCreatesOwnerOnlyLogFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not provide Unix file-mode semantics")
	}

	restoreDefaultLogger(t)
	logPath := filepath.Join(t.TempDir(), "git-auto-sync.log")

	_, logCloser, err := setupLoggerWithPathAndOutput(false, logPath, io.Discard)
	if err != nil {
		t.Fatalf("setupLoggerWithPathAndOutput() error = %v, want nil", err)
	}
	if logCloser == nil {
		t.Fatal("setupLoggerWithPathAndOutput() closer is nil")
	}
	t.Cleanup(func() {
		if closeErr := logCloser.Close(); closeErr != nil {
			t.Errorf("closing log writer: %v", closeErr)
		}
	})

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", logPath, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("log file mode = %04o, want 0600", got)
	}
}

// @description    Verifies setup restricts an existing Unix log file to owner-only permissions.
//
// @param           t  "test handle used for temporary file and mode assertions"
func TestSetupLoggerWithPathRestrictsExistingLogFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not provide Unix file-mode semantics")
	}

	restoreDefaultLogger(t)
	logPath := filepath.Join(t.TempDir(), "git-auto-sync.log")
	if err := os.WriteFile(logPath, []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", logPath, err)
	}
	if err := os.Chmod(logPath, 0o644); err != nil {
		t.Fatalf("Chmod(%q) error = %v", logPath, err)
	}

	_, logCloser, err := setupLoggerWithPathAndOutput(false, logPath, io.Discard)
	if err != nil {
		t.Fatalf("setupLoggerWithPathAndOutput() error = %v, want nil", err)
	}
	if logCloser == nil {
		t.Fatal("setupLoggerWithPathAndOutput() closer is nil")
	}
	t.Cleanup(func() {
		if closeErr := logCloser.Close(); closeErr != nil {
			t.Errorf("closing log writer: %v", closeErr)
		}
	})

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", logPath, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("existing log file mode = %04o, want 0600", got)
	}
}

// @description    Verifies debug setup fans each record out to both the log file and stdout.
//
// @param           t  "test handle used for fan-out assertions and logger cleanup"
func TestSetupLoggerWithPathDebugFansOutToFileAndStdout(t *testing.T) {
	restoreDefaultLogger(t)
	logPath := filepath.Join(t.TempDir(), "git-auto-sync.log")
	var stdout bytes.Buffer

	logger, logCloser, err := setupLoggerWithPathAndOutput(true, logPath, &stdout)
	if err != nil {
		t.Fatalf("setupLoggerWithPathAndOutput() error = %v, want nil", err)
	}
	if logCloser == nil {
		t.Fatal("setupLoggerWithPathAndOutput() closer is nil")
	}
	t.Cleanup(func() {
		if closeErr := logCloser.Close(); closeErr != nil {
			t.Errorf("closing log writer: %v", closeErr)
		}
	})

	logger.Debug("fan-out record", "repo", "example")
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", logPath, err)
	}
	for name, output := range map[string]string{"file": string(contents), "stdout": stdout.String()} {
		for _, substring := range []string{"level=DEBUG", "msg=\"fan-out record\"", "repo=example"} {
			if !strings.Contains(output, substring) {
				t.Errorf("%s output %q does not contain %q", name, output, substring)
			}
		}
	}
}

// @description    Verifies debug fallback writes DEBUG records to stdout without returning an error.
//
// @param           t  "test handle used for fallback stdout assertions and global cleanup"
func TestSetupLoggerWithPathDebugFallbackWritesStdout(t *testing.T) {
	restoreDefaultLogger(t)
	originalStderr := os.Stderr
	t.Cleanup(func() { os.Stderr = originalStderr })
	warningFile, err := os.CreateTemp(t.TempDir(), "stderr-*.log")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = warningFile.Close() })
	os.Stderr = warningFile

	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", parentFile, err)
	}
	var stdout bytes.Buffer
	logger, logCloser, setupErr := setupLoggerWithPathAndOutput(true, filepath.Join(parentFile, "git-auto-sync.log"), &stdout)
	if setupErr != nil {
		t.Fatalf("setupLoggerWithPathAndOutput() error = %v, want nil fallback", setupErr)
	}
	if logCloser != nil {
		t.Fatal("setupLoggerWithPathAndOutput() fallback closer is nonnil")
	}

	logger.Debug("fallback record", "repo", "example")
	output := stdout.String()
	for _, substring := range []string{"level=DEBUG", "msg=\"fallback record\"", "repo=example"} {
		if !strings.Contains(output, substring) {
			t.Errorf("fallback stdout %q does not contain %q", output, substring)
		}
	}
}

// @description    Verifies unusable log paths fall back without returning a setup error.
//
// @param           t  "test handle used for fallback assertions and global cleanup"
func TestSetupLoggerWithPathFallsBackForUnusablePath(t *testing.T) {
	restoreDefaultLogger(t)
	originalStderr := os.Stderr
	t.Cleanup(func() { os.Stderr = originalStderr })

	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", parentFile, err)
	}
	warningFile, err := os.CreateTemp(t.TempDir(), "stderr-*.log")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	os.Stderr = warningFile

	logger, setupErr := setupLoggerWithPath(false, filepath.Join(parentFile, "git-auto-sync.log"))
	os.Stderr = originalStderr
	if closeErr := warningFile.Close(); closeErr != nil {
		t.Fatalf("closing captured stderr: %v", closeErr)
	}

	if setupErr != nil {
		t.Fatalf("setupLoggerWithPath() error = %v, want nil fallback", setupErr)
	}
	if logger == nil {
		t.Fatal("setupLoggerWithPath() fallback logger is nil")
	}
	if slog.Default() != logger {
		t.Fatal("setupLoggerWithPath() did not install fallback logger as default")
	}
	warning, err := os.ReadFile(warningFile.Name())
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", warningFile.Name(), err)
	}
	if got := string(warning); !strings.Contains(got, "warning: cannot open log file:") ||
		!strings.Contains(got, "not-a-directory") {
		t.Fatalf("fallback warning = %q, want meaningful path warning", got)
	}
}

// @description    Verifies CLI and daemon log paths use distinct fixed filenames.
//
// @param           t  "test handle used for executable-specific path assertions"
func TestLogPathForPlatformUsesExecutableFilename(t *testing.T) {
	logDir := filepath.Join("root", "home", ".local", "share", "git-auto-sync", "log")
	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "CLI",
			filename: cliLogFilename,
			want:     filepath.Join(logDir, "git-auto-sync.log"),
		},
		{
			name:     "daemon",
			filename: daemonLogFilename,
			want:     filepath.Join(logDir, "git-auto-sync-daemon.log"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := logPathForPlatform("linux", filepath.Join("root", "home"), "", tt.filename)
			if got != tt.want {
				t.Fatalf("logPathForPlatform() = %q, want %q", got, tt.want)
			}
		})
	}
}

// @description    Verifies platform-specific log directory construction from explicit inputs.
//
// @param           t  "test handle used for platform path table assertions"
func TestLogDirForPlatform(t *testing.T) {
	home := filepath.Join("root", "home")
	localAppData := filepath.Join("windows", "local")
	tests := []struct {
		name         string
		goos         string
		home         string
		localAppData string
		want         string
	}{
		{
			name: "linux",
			goos: "linux",
			home: home,
			want: filepath.Join(home, ".local", "share", "git-auto-sync", "log"),
		},
		{
			name: "darwin",
			goos: "darwin",
			home: home,
			want: filepath.Join(home, "Library", "Logs"),
		},
		{
			name:         "windows with LOCALAPPDATA",
			goos:         "windows",
			home:         home,
			localAppData: localAppData,
			want:         filepath.Join(localAppData, "git-auto-sync", "logs"),
		},
		{
			name: "windows home fallback",
			goos: "windows",
			home: home,
			want: filepath.Join(home, "AppData", "Local", "git-auto-sync", "logs"),
		},
		{
			name:         "windows LOCALAPPDATA without home",
			goos:         "windows",
			localAppData: localAppData,
			want:         filepath.Join(localAppData, "git-auto-sync", "logs"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logDirForPlatform(tt.goos, tt.home, tt.localAppData); got != tt.want {
				t.Fatalf("logDirForPlatform(%q, %q, %q) = %q, want %q", tt.goos, tt.home, tt.localAppData, got, tt.want)
			}
		})
	}
}

// recordingHandler captures handled records and optionally returns an error.
type recordingHandler struct {
	level  slog.Level
	err    error
	buffer bytes.Buffer
	calls  int
}

// @description    Creates a record-capturing handler with a minimum level and configured error.
//
// @param           level  "minimum enabled record level"
//
// @param           err    "error returned after recording a handled record"
//
// @return          *recordingHandler  "configured record-capturing handler"
func newRecordingHandler(level slog.Level, err error) *recordingHandler {
	return &recordingHandler{level: level, err: err}
}

// @description    Reports whether the recording handler accepts a level.
//
// @param           _      "unused context"
//
// @param           level  "record level to check"
//
// @return          bool   "true when the level meets the configured minimum"
func (h *recordingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// @description    Records a log record and returns the configured error.
//
// @param           _       "unused context"
//
// @param           record  "record to capture"
//
// @return          error   "configured handler error"
func (h *recordingHandler) Handle(_ context.Context, record slog.Record) error {
	h.calls++
	if err := slog.NewTextHandler(&h.buffer, &slog.HandlerOptions{ReplaceAttr: removeTimeAttr}).Handle(context.Background(), record); err != nil {
		return err
	}
	return h.err
}

// @description    Returns the recording handler because tests do not derive it directly.
//
// @param           _  "unused attributes"
//
// @return          slog.Handler  "the same recording handler"
func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

// @description    Returns the recording handler because tests do not group it directly.
//
// @param           _  "unused group name"
//
// @return          slog.Handler  "the same recording handler"
func (h *recordingHandler) WithGroup(_ string) slog.Handler {
	return h
}

// @description    Returns the number of records handled by the recording handler.
//
// @return          int  "handled record count"
func (h *recordingHandler) callCount() int {
	return h.calls
}

// @description    Returns captured text output from the recording handler.
//
// @return          string  "captured record text"
func (h *recordingHandler) output() string {
	return h.buffer.String()
}

// @description    Removes the time attribute to keep captured output deterministic.
//
// @param           _     "unused attribute group path"
//
// @param           attr  "attribute being rendered"
//
// @return          slog.Attr  "empty attribute for time, otherwise the original attribute"
func removeTimeAttr(_ []string, attr slog.Attr) slog.Attr {
	if attr.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return attr
}

// @description    Restores slog's process-wide default logger when a test completes.
//
// @param           t  "test handle used to register cleanup"
func restoreDefaultLogger(t *testing.T) {
	t.Helper()
	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })
}
