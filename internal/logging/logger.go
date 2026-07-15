package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	cliLogFilename    = "git-auto-sync.log"
	daemonLogFilename = "git-auto-sync-daemon.log"
)

// @description    Initializes the shared CLI logger.
//
// SetupLogger configures the CLI's rotating log file and, when debug is enabled, an additional
// stdout handler. It shares all logger behavior with SetupDaemonLogger and installs the result as
// slog's default logger.
//
// @param           debug  "when true, also emit logs to stdout at DEBUG level"
//
// @return          *slog.Logger  "configured logger"
//
// @return          error         "always nil; setup failures degrade instead of returning an error"
func SetupLogger(debug bool) (*slog.Logger, error) {
	return setupLoggerWithPath(debug, defaultLogPath(cliLogFilename))
}

// @description    Initializes the shared daemon logger.
//
// SetupDaemonLogger configures the daemon's rotating log file and, when debug is enabled, an
// additional stdout handler. It shares all logger behavior with SetupLogger and installs the result
// as slog's default logger.
//
// @param           debug  "when true, also emit logs to stdout at DEBUG level"
//
// @return          *slog.Logger  "configured logger"
//
// @return          error         "always nil; setup failures degrade instead of returning an error"
func SetupDaemonLogger(debug bool) (*slog.Logger, error) {
	return setupLoggerWithPath(debug, defaultLogPath(daemonLogFilename))
}

// @description    Initializes the logger with an explicit log file path.
//
// setupLoggerWithPath exists so tests can exercise logger setup without touching the real log
// directory.
//
// @param           debug    "when true, also emit logs to stdout at DEBUG level"
//
// @param           logPath  "full path to the log file"
//
// @return          *slog.Logger  "configured logger"
//
// @return          error         "always nil; setup failures degrade instead of returning an error"
func setupLoggerWithPath(debug bool, logPath string) (*slog.Logger, error) {
	logger, _, err := setupLoggerWithPathAndOutput(debug, logPath, os.Stdout)
	return logger, err
}

// @description    Initializes the logger with injectable debug output and exposes file cleanup, set default logger.
//
// setupLoggerWithPathAndOutput keeps production logger signatures stable while allowing tests to
// close lumberjack's lazily opened file before temporary-directory cleanup.
//
// @param           debug        "when true, also emit logs to the debug output at DEBUG level"
//
// @param           logPath      "full path to the log file"
//
// @param           debugOutput  "destination used for debug console output"
//
// @return          *slog.Logger  "configured logger"
//
// @return          io.Closer     "file writer to close, or nil when setup falls back"
//
// @return          error         "always nil; setup failures degrade instead of returning an error"
func setupLoggerWithPathAndOutput(debug bool, logPath string, debugOutput io.Writer) (*slog.Logger, io.Closer, error) {
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		logger, fallbackErr := fallbackLoggerWithOutput(debug, fmt.Errorf("create log directory %q: %w", logDir, err), debugOutput)
		return logger, nil, fallbackErr
	}

	pathInfo, err := os.Lstat(logPath)
	if err == nil {
		if pathInfo.Mode()&os.ModeSymlink != 0 {
			logger, fallbackErr := fallbackLoggerWithOutput(debug, fmt.Errorf("log file %q is a symbolic link", logPath), debugOutput)
			return logger, nil, fallbackErr
		}
		if !pathInfo.Mode().IsRegular() {
			logger, fallbackErr := fallbackLoggerWithOutput(debug, fmt.Errorf("log file %q is not a regular file", logPath), debugOutput)
			return logger, nil, fallbackErr
		}
	} else if !os.IsNotExist(err) {
		logger, fallbackErr := fallbackLoggerWithOutput(debug, fmt.Errorf("inspect log file %q: %w", logPath, err), debugOutput)
		return logger, nil, fallbackErr
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		logger, fallbackErr := fallbackLoggerWithOutput(debug, fmt.Errorf("open log file %q: %w", logPath, err), debugOutput)
		return logger, nil, fallbackErr
	}
	if runtime.GOOS != "windows" {
		info, statErr := logFile.Stat()
		if statErr != nil {
			_ = logFile.Close()
			logger, fallbackErr := fallbackLoggerWithOutput(debug, fmt.Errorf("stat log file %q: %w", logPath, statErr), debugOutput)
			return logger, nil, fallbackErr
		}
		if !info.Mode().IsRegular() {
			_ = logFile.Close()
			logger, fallbackErr := fallbackLoggerWithOutput(debug, fmt.Errorf("log file %q is not a regular file", logPath), debugOutput)
			return logger, nil, fallbackErr
		}
		if chmodErr := logFile.Chmod(0o600); chmodErr != nil {
			_ = logFile.Close()
			logger, fallbackErr := fallbackLoggerWithOutput(debug, fmt.Errorf("restrict log file %q permissions: %w", logPath, chmodErr), debugOutput)
			return logger, nil, fallbackErr
		}
	}
	if err := logFile.Close(); err != nil {
		logger, fallbackErr := fallbackLoggerWithOutput(debug, fmt.Errorf("close log file %q: %w", logPath, err), debugOutput)
		return logger, nil, fallbackErr
	}

	fileWriter := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10,
		MaxBackups: 3,
		LocalTime:  true,
	}

	handlerOptions := &slog.HandlerOptions{Level: slog.LevelDebug}
	handlers := []slog.Handler{slog.NewTextHandler(fileWriter, handlerOptions)}
	if debug {
		handlers = append(handlers, slog.NewTextHandler(debugOutput, handlerOptions))
	}

	logger := slog.New(&multiHandler{handlers: handlers})
	slog.SetDefault(logger)
	return logger, fileWriter, nil
}

// @description    Builds a degraded logger with an injectable debug output destination, set default logger.
//
// @param           debug        "when true, log to the debug output; otherwise discard logs"
//
// @param           err          "error that triggered fallback"
//
// @param           debugOutput  "destination used when debug logging is enabled"
//
// @return          *slog.Logger  "degraded logger"
//
// @return          error         "always nil"
func fallbackLoggerWithOutput(debug bool, err error, debugOutput io.Writer) (*slog.Logger, error) {
	_, _ = fmt.Fprintf(os.Stderr, "warning: cannot open log file: %v\n", err)

	output := io.Writer(io.Discard)
	if debug {
		output = debugOutput
	}
	logger := slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	return logger, nil
}

// @description    Returns a named default log file path for the current platform.
//
// defaultLogPath reads platform and environment state before delegating path construction to
// logPathForPlatform. On Windows, LOCALAPPDATA is sufficient even when no user home is available.
// The function falls back to the current directory when neither the required environment value nor
// a user home can be determined.
//
// @param           filename  "fixed executable-specific log filename"
//
// @return          string    "platform-appropriate full log file path"
func defaultLogPath(filename string) string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if runtime.GOOS == "windows" && localAppData != "" {
		return logPathForPlatform(runtime.GOOS, "", localAppData, filename)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", filename)
	}
	return logPathForPlatform(runtime.GOOS, home, localAppData, filename)
}

// @description    Builds a named log path from explicit platform path inputs.
//
// logPathForPlatform is side-effect free so executable-specific path selection can be tested
// independently of the host operating system, process environment, and real user home.
//
// @param           goos          "target operating-system identifier"
//
// @param           home          "user home directory"
//
// @param           localAppData  "Windows local application-data directory, when available"
//
// @param           filename      "fixed executable-specific log filename"
//
// @return          string        "platform-appropriate full log file path"
func logPathForPlatform(goos, home, localAppData, filename string) string {
	return filepath.Join(logDirForPlatform(goos, home, localAppData), filename)
}

// @description    Builds a log directory from explicit platform path inputs.
//
// logDirForPlatform is side-effect free so platform path selection can be tested independently of
// the host operating system and process environment.
//
// @param           goos          "target operating-system identifier"
//
// @param           home          "user home directory"
//
// @param           localAppData  "Windows local application-data directory, when available"
//
// @return          string        "platform-appropriate log directory"
func logDirForPlatform(goos, home, localAppData string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Logs")
	case "windows":
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(localAppData, "git-auto-sync", "logs")
	default:
		return filepath.Join(home, ".local", "share", "git-auto-sync", "log")
	}
}

// multiHandler dispatches log records to multiple child handlers.
type multiHandler struct {
	handlers []slog.Handler
}

// @description    Reports whether any child handler is enabled for the given level.
//
// @param           ctx    "context for the log record"
//
// @param           level  "level to check"
//
// @return          bool   "true if at least one child handler is enabled"
func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range m.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// @description    Dispatches a log record to every enabled child handler.
//
// Handle preserves the first child error while continuing to dispatch the record to later enabled
// handlers.
//
// @param           ctx  "context for the log record"
//
// @param           r    "record to dispatch"
//
// @return          error  "first child-handler error, or nil"
func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, handler := range m.handlers {
		if !handler.Enabled(ctx, r.Level) {
			continue
		}
		if err := handler.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// @description    Returns a new handler with attributes applied to every child.
//
// @param           attrs  "attributes to add"
//
// @return          slog.Handler  "new multi-handler containing the derived child handlers"
func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, handler := range m.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

// @description    Returns a new handler with a group applied to every child.
//
// @param           name  "group name"
//
// @return          slog.Handler  "new multi-handler containing the grouped child handlers"
func (m *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(m.handlers))
	for i, handler := range m.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}
