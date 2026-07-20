package daemonservice

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kardianos/service"

	"github.com/northhalf/git-auto-sync/internal/termux"
)

const (
	managedMarkerName    = ".git-auto-sync-managed"
	managedMarkerContent = "git-auto-sync-runit-v1"
	daemonServiceName    = "git-auto-sync-daemon"
)

type runCommand func(string, ...string) ([]byte, error)

type runitBackend struct {
	prefix      string
	svPath      string
	serviceRoot string
	serviceDir  string
	loggerPath  string
	daemonPath  string
	run         runCommand
}

// @description    Creates a validated Termux runit service backend.
//
// @param           prefix      "absolute Termux PREFIX path"
//
// @param           daemonPath  "absolute daemon executable path"
//
// @param           runner      "command runner used for sv lifecycle operations"
//
// @return          *runitBackend  "validated runit backend"
//
// @return          error          "invalid path or missing dependency error"
func newRunitBackend(prefix, daemonPath string, runner runCommand) (*runitBackend, error) {
	if prefix == "" || !filepath.IsAbs(prefix) {
		return nil, errors.New("termux PREFIX must be an absolute path")
	}
	backend := &runitBackend{
		prefix:      prefix,
		svPath:      filepath.Join(prefix, "bin", "sv"),
		serviceRoot: filepath.Join(prefix, "var", "service"),
		serviceDir:  filepath.Join(prefix, "var", "service", daemonServiceName),
		loggerPath:  filepath.Join(prefix, "share", "termux-services", "svlogger"),
		daemonPath:  daemonPath,
		run:         runner,
	}
	if err := termux.RequireExecutable(backend.svPath); err != nil {
		return nil, termuxServicesError(err)
	}
	serviceRootInfo, err := os.Stat(backend.serviceRoot)
	if err != nil {
		return nil, termuxServicesError(err)
	}
	if !serviceRootInfo.IsDir() {
		return nil, termuxServicesError(fmt.Errorf("%s is not a directory", backend.serviceRoot))
	}
	loggerInfo, err := os.Stat(backend.loggerPath)
	if err != nil {
		return nil, termuxServicesError(err)
	}
	if !loggerInfo.Mode().IsRegular() {
		return nil, termuxServicesError(fmt.Errorf("%s is not a regular file", backend.loggerPath))
	}
	if err := termux.RequireExecutable(backend.daemonPath); err != nil {
		return nil, fmt.Errorf("git-auto-sync-daemon is unavailable: %w", err)
	}
	return backend, nil
}

// @description    Wraps a missing runit dependency with Termux installation guidance.
//
// @param           cause  "underlying filesystem or executable error"
//
// @return          error  "dependency error containing installation and startup commands"
func termuxServicesError(cause error) error {
	return fmt.Errorf("termux daemon management requires termux-services: %w\ninstall it with: pkg install termux-services\nthen restart Termux or run:\nsource \"$PREFIX/etc/profile.d/start-services.sh\"", cause)
}

// @description    Adds Termux service-supervisor startup guidance to an sv error.
//
// @param           cause  "underlying sv command failure"
//
// @return          error  "runtime error containing service-daemon startup guidance"
func termuxSupervisorError(cause error) error {
	return fmt.Errorf("termux service supervisor is unavailable: %w\nrestart Termux or run:\nsource \"$PREFIX/etc/profile.d/start-services.sh\"", cause)
}

// @description    Installs or updates the managed runit service definition.
//
// @return          error  "nil on success, or a service ownership or filesystem error"
func (r *runitBackend) Install() error {
	managed, exists, err := r.managedState()
	if err != nil {
		return err
	}
	if exists && !managed {
		return fmt.Errorf("refusing to overwrite unmanaged runit service: %s", r.serviceDir)
	}
	if exists {
		return r.writeManagedFiles(r.serviceDir)
	}

	tempDir, err := os.MkdirTemp(r.serviceRoot, ".git-auto-sync-daemon-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	if err := r.writeManagedFiles(tempDir); err != nil {
		return err
	}
	return os.Rename(tempDir, r.serviceDir)
}

// @description    Writes the marker, run script, and logger link into a service directory.
//
// @param           dir  "service directory to populate"
//
// @return          error  "nil on success, or a filesystem error"
func (r *runitBackend) writeManagedFiles(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "log"), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(dir, managedMarkerName), []byte(managedMarkerContent+"\n"), 0o644); err != nil {
		return err
	}
	runScript := "#!" + filepath.Join(r.prefix, "bin", "sh") + "\nexec 2>&1\nexec " + shellQuote(r.daemonPath) + "\n"
	if err := writeFileAtomic(filepath.Join(dir, "run"), []byte(runScript), 0o755); err != nil {
		return err
	}
	loggerLink := filepath.Join(dir, "log", "run")
	if err := os.Remove(loggerLink); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Symlink(r.loggerPath, loggerLink)
}

// @description    Writes a file through a same-directory temporary file and atomic rename.
//
// @param           path  "destination file path"
//
// @param           data  "complete file contents"
//
// @param           mode  "destination permission bits"
//
// @return          error  "nil on success, or a filesystem error"
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

// @description    Quotes a string for one POSIX shell argument.
//
// @param           value  "untrusted argument value"
//
// @return          string  "single-quoted shell representation"
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// @description    Reports whether the service directory exists and carries the managed marker.
//
// @return          bool   "true when the marker matches the supported format"
//
// @return          bool   "true when the service directory exists"
//
// @return          error  "filesystem error other than absence"
func (r *runitBackend) managedState() (bool, bool, error) {
	info, err := os.Stat(r.serviceDir)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if !info.IsDir() {
		return false, true, nil
	}
	marker, err := os.ReadFile(filepath.Join(r.serviceDir, managedMarkerName))
	if errors.Is(err, os.ErrNotExist) {
		return false, true, nil
	}
	if err != nil {
		return false, true, err
	}
	return strings.TrimSuffix(string(marker), "\n") == managedMarkerContent, true, nil
}

// @description    Returns the managed runit service status.
//
// @return          service.Status  "running, stopped, or unknown status"
//
// @return          error           "ErrNotInstalled, ownership failure, or sv command error"
func (r *runitBackend) Status() (service.Status, error) {
	managed, exists, err := r.managedState()
	if err != nil {
		return service.StatusUnknown, err
	}
	if !exists {
		return service.StatusUnknown, ErrNotInstalled
	}
	if !managed {
		return service.StatusUnknown, fmt.Errorf("unmanaged runit service: %s", r.serviceDir)
	}
	output, err := r.run(r.svPath, "status", r.serviceDir)
	if err != nil {
		return service.StatusUnknown, termuxSupervisorError(err)
	}
	status := strings.TrimSpace(string(output))
	switch {
	case strings.HasPrefix(status, "run:"):
		return service.StatusRunning, nil
	case strings.HasPrefix(status, "down:"):
		return service.StatusStopped, nil
	default:
		return service.StatusUnknown, fmt.Errorf("unknown sv status output: %s", status)
	}
}

// @description    Starts the managed runit service.
//
// @return          error  "nil on success, or an sv command error"
func (r *runitBackend) Start() error {
	_, err := r.run(r.svPath, "up", r.serviceDir)
	if err != nil {
		return termuxSupervisorError(err)
	}
	return nil
}

// @description    Stops the managed runit service.
//
// @return          error  "nil on success, or an sv command error"
func (r *runitBackend) Stop() error {
	_, err := r.run(r.svPath, "down", r.serviceDir)
	if err != nil {
		return termuxSupervisorError(err)
	}
	return nil
}

// @description    Removes the managed runit service directory.
//
// @return          error  "nil on success, or an ownership or filesystem error"
func (r *runitBackend) Uninstall() error {
	managed, exists, err := r.managedState()
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotInstalled
	}
	if !managed {
		return fmt.Errorf("refusing to remove unmanaged runit service: %s", r.serviceDir)
	}
	return os.RemoveAll(r.serviceDir)
}
