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
func SetupLogger(debug bool) *slog.Logger {
	logger, _ := setupLoggerWithPath(debug, defaultLogPath(cliLogFilename))
	return logger
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
func SetupDaemonLogger(debug bool) *slog.Logger {
	logger, _ := setupLoggerWithPath(debug, defaultLogPath(daemonLogFilename))
	return logger
}

// @description    Initializes the logger at the given path, installs it as the default, and exposes file cleanup.
//
// setupLoggerWithPath keeps production logger signatures stable while allowing tests to
// close lumberjack's lazily opened file before temporary-directory cleanup. Debug console output
// always goes to stdout, and setup failures degrade to fallbackLogger instead of returning an
// error.
//
// @param           debug    "when true, also emit logs to stdout at DEBUG level"
//
// @param           logPath  "full path to the log file"
//
// @return          *slog.Logger  "configured logger"
//
// @return          io.Closer     "file writer to close, or nil when setup falls back"
func setupLoggerWithPath(debug bool, logPath string) (*slog.Logger, io.Closer) {
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fallbackLogger(debug, fmt.Errorf("create log directory %q: %w", logDir, err)), nil
	}

	pathInfo, err := os.Lstat(logPath)
	if err == nil {
		if pathInfo.Mode()&os.ModeSymlink != 0 {
			return fallbackLogger(debug, fmt.Errorf("log file %q is a symbolic link", logPath)), nil
		}
		if !pathInfo.Mode().IsRegular() {
			return fallbackLogger(debug, fmt.Errorf("log file %q is not a regular file", logPath)), nil
		}
	} else if !os.IsNotExist(err) {
		return fallbackLogger(debug, fmt.Errorf("inspect log file %q: %w", logPath, err)), nil
	}

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fallbackLogger(debug, fmt.Errorf("open log file %q: %w", logPath, err)), nil
	}
	if runtime.GOOS != "windows" {
		info, statErr := logFile.Stat()
		if statErr != nil {
			_ = logFile.Close()
			return fallbackLogger(debug, fmt.Errorf("stat log file %q: %w", logPath, statErr)), nil
		}
		if !info.Mode().IsRegular() {
			_ = logFile.Close()
			return fallbackLogger(debug, fmt.Errorf("log file %q is not a regular file", logPath)), nil
		}
		if chmodErr := logFile.Chmod(0o600); chmodErr != nil {
			_ = logFile.Close()
			return fallbackLogger(debug, fmt.Errorf("restrict log file %q permissions: %w", logPath, chmodErr)), nil
		}
	}
	if err := logFile.Close(); err != nil {
		return fallbackLogger(debug, fmt.Errorf("close log file %q: %w", logPath, err)), nil
	}

	fileWriter := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    10,
		MaxBackups: 3,
		LocalTime:  true,
	}

	handlerOptions := newHandlerOptions(debug)
	handlers := []slog.Handler{slog.NewTextHandler(fileWriter, handlerOptions)}
	if debug {
		handlers = append(handlers, slog.NewTextHandler(os.Stdout, handlerOptions))
	}

	logger := slog.New(&multiHandler{handlers: handlers})
	slog.SetDefault(logger)
	return logger, fileWriter
}

// @description    Builds a degraded logger that warns on stderr, set default logger.
//
// @param           debug  "when true, log to stdout; otherwise discard logs"
//
// @param           err    "error that triggered fallback"
//
// @return          *slog.Logger  "degraded logger"
func fallbackLogger(debug bool, err error) *slog.Logger {
	_, _ = fmt.Fprintf(os.Stderr, "warning: cannot open log file: %v\n", err)

	output := io.Writer(io.Discard)
	if debug {
		output = os.Stdout
	}
	logger := slog.New(slog.NewTextHandler(output, newHandlerOptions(debug)))
	slog.SetDefault(logger)
	return logger
}

// @description    Returns a logger with the repository path attached to every record.
//
// @param           repoPath  "path to the repository root"
//
// @return          *slog.Logger  "default logger derived with the repository attribute"
func WithRepo(repoPath string) *slog.Logger {
	return slog.Default().With("repo", repoPath)
}

// @description    Builds text-handler options shared by file, console, and fallback logging.
//
// @param           debug  "when true, include the source file and line"
//
// @return          *slog.HandlerOptions  "handler options with DEBUG level and shortened timestamps"
func newHandlerOptions(debug bool) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: debug,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case slog.TimeKey:
				return slog.String(slog.TimeKey, attr.Value.Time().Format("2006-01-02T15:04:05"))
			case slog.SourceKey:
				source, ok := attr.Value.Any().(*slog.Source)
				if !ok {
					return attr
				}
				return slog.String(slog.SourceKey, filepath.Base(source.File))
			default:
				return attr
			}
		},
	}
}

// @description    Returns a named default log file path for the current platform.
//
// defaultLogPath reads platform and environment state before joining the log directory with the
// file name. On Windows, LOCALAPPDATA is sufficient even when no user home is available. The
// function falls back to the current directory when neither the required environment value nor a
// user home can be determined.
//
// @param           filename  "fixed executable-specific log filename"
//
// @return          string    "platform-appropriate full log file path"
func defaultLogPath(filename string) string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if runtime.GOOS == "windows" && localAppData != "" {
		return filepath.Join(logDirForPlatform(runtime.GOOS, "", localAppData), filename)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", filename)
	}
	return filepath.Join(logDirForPlatform(runtime.GOOS, home, localAppData), filename)
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
